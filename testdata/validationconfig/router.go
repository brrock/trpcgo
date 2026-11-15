package validationconfig

import (
	"context"

	"github.com/brrock/trpcgo"
)

func Register(r *trpcgo.Router) {
	trpcgo.MustQuery(r, "config.input", func(context.Context, Input) (bool, error) { return true, nil })
}
