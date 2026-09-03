package route

import (
	"github.com/apptivitypl/rill"

	"github.com/apptivitypl/rill/examples/hello-world/server/hackernews"
)

func GET(ctx *rill.Ctx, params rill.Params) (rill.Response, error) {
	stories, _ := hackernews.Top(ctx.Request())
	return rill.JSON(stories), nil
}
