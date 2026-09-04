package route

import (
	"github.com/apptivitypl/gopage"

	"github.com/apptivitypl/gopage/examples/hello-world/server/hackernews"
)

func GET(ctx *gopage.Ctx, params gopage.Params) (gopage.Response, error) {
	stories, _ := hackernews.Top(ctx.Request())
	return gopage.JSON(stories), nil
}
