package route

import "github.com/apptivitypl/gopage"

type Status struct {
	Status  string `json:"status"`
	Runtime string `json:"runtime"`
}

func GET(ctx *gopage.Ctx, params gopage.Params) (gopage.Response, error) {
	return gopage.JSON(Status{Status: "ok", Runtime: "go"}), nil
}
