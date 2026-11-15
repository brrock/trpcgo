package trpc

import (
	"cmp"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"

	"github.com/brrock/trpcgo"
)

// parsedRequest represents a single procedure call extracted from an HTTP request.
type parsedRequest struct {
	path  string
	input json.RawMessage
}

// containsTraversal reports whether path has a "." or ".." segment, splitting
// on "/" and on the "," that separates batched procedures.
func containsTraversal(path string) bool {
	return slices.ContainsFunc(strings.FieldsFunc(path, func(r rune) bool { return r == '/' || r == ',' }), func(segment string) bool {
		return segment == "." || segment == ".."
	})
}

// parseRequest extracts procedure path(s) and input(s) from an HTTP request.
func parseRequest(r *http.Request, isBatch bool, maxBodySize int64) ([]parsedRequest, error) {
	if containsTraversal(r.URL.Path) {
		return nil, trpcgo.NewError(trpcgo.CodeBadRequest, "invalid procedure path")
	}
	rawPath := procedurePath(r)
	if rawPath == "" {
		return nil, trpcgo.NewError(trpcgo.CodeNotFound, "no procedure path specified")
	}

	if !isBatch {
		input, err := parseInput(r, maxBodySize)
		if err != nil {
			return nil, err
		}
		return []parsedRequest{{path: rawPath, input: input}}, nil
	}

	// Batch: paths are comma-separated.
	paths := strings.Split(rawPath, ",")
	var indexedInputs map[string]json.RawMessage
	if r.Method == http.MethodGet {
		rawInput := r.URL.Query().Get("input")
		if maxBodySize > 0 && int64(len(rawInput)) > maxBodySize {
			return nil, trpcgo.NewError(trpcgo.CodePayloadTooLarge, "query input too large")
		}
		if rawInput != "" {
			if err := json.Unmarshal([]byte(rawInput), &indexedInputs); err != nil {
				return nil, trpcgo.NewError(trpcgo.CodeParseError, "failed to parse batch input")
			}
		}
	} else {
		body, err := readBody(r, maxBodySize)
		if err != nil {
			return nil, err
		}
		if len(body) > 0 {
			if err := json.Unmarshal(body, &indexedInputs); err != nil {
				return nil, trpcgo.NewError(trpcgo.CodeParseError, "failed to parse batch input")
			}
		}
	}

	results := make([]parsedRequest, len(paths))
	for i, path := range paths {
		results[i] = parsedRequest{
			path:  path,
			input: indexedInputs[fmt.Sprintf("%d", i)],
		}
	}
	return results, nil
}

// parseInput extracts the input for a single (non-batch) procedure call.
func parseInput(r *http.Request, maxBodySize int64) (json.RawMessage, error) {
	if r.Method == http.MethodGet {
		rawInput := r.URL.Query().Get("input")
		if rawInput == "" {
			return nil, nil
		}
		if maxBodySize > 0 && int64(len(rawInput)) > maxBodySize {
			return nil, trpcgo.NewError(trpcgo.CodePayloadTooLarge, "query input too large")
		}
		return json.RawMessage(rawInput), nil
	}
	body, err := readBody(r, maxBodySize)
	if err != nil {
		return nil, err
	}
	if len(body) == 0 {
		return nil, nil
	}
	return json.RawMessage(body), nil
}

// readBody reads the request body with a size limit.
func readBody(r *http.Request, maxSize int64) ([]byte, error) {
	reader := r.Body
	if maxSize > 0 {
		reader = http.MaxBytesReader(nil, r.Body, maxSize)
	}
	body, err := io.ReadAll(reader)
	if err != nil {
		return nil, trpcgo.NewError(trpcgo.CodePayloadTooLarge, "request body too large")
	}
	return body, nil
}

// isBatchRequest reports whether the request carries ?batch=1.
func isBatchRequest(r *http.Request) bool {
	return r.URL.Query().Get("batch") == "1"
}

// parsePaths extracts procedure paths from the URL without parsing inputs.
func parsePaths(r *http.Request) []string {
	rawPath := procedurePath(r)
	if rawPath == "" {
		return nil
	}
	return strings.Split(rawPath, ",")
}

// procedurePath returns the last segment of the URL path. Procedure paths
// never contain "/", so the handler works wherever it is mounted.
func procedurePath(r *http.Request) string {
	path := r.URL.Path
	return path[strings.LastIndex(path, "/")+1:]
}

// mergeLastEventId reads the lastEventId from the request and merges it into input.
func mergeLastEventId(r *http.Request, input json.RawMessage) json.RawMessage {
	lastEventId := r.Header.Get("Last-Event-Id")
	if lastEventId == "" {
		query := r.URL.Query()
		lastEventId = cmp.Or(query.Get("lastEventId"), query.Get("Last-Event-Id"))
	}
	if lastEventId == "" {
		return input
	}
	var obj map[string]json.RawMessage
	if len(input) > 0 {
		if err := json.Unmarshal(input, &obj); err != nil {
			return input
		}
	}
	if obj == nil {
		obj = make(map[string]json.RawMessage)
	}
	idVal, _ := json.Marshal(lastEventId)
	obj["lastEventId"] = idVal
	merged, _ := json.Marshal(obj)
	return merged
}
