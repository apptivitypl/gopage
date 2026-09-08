package runtime

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/apptivitypl/gopage/internal/i18n"
	"github.com/apptivitypl/gopage/internal/ir"
)

const (
	CountArgument    = "count"
	CountPlaceholder = "{" + CountArgument + "}"
)

func (s *scope) message(node ir.ExprNode) (Value, error) {
	form := i18n.FormOther
	countText := ""
	if node.B != NoArgument {
		value, err := s.eval(node.B)
		if err != nil {
			return Nil(), err
		}
		number, numeric := value.Number()
		form = s.formOf(number)
		countText = formatCount(value, number, numeric)
	}
	text, ok := s.messageText(node.A, form)
	if !ok {
		return String(s.plan.Message(node.A)), nil
	}
	if countText == "" {
		return String(text), nil
	}
	return String(strings.ReplaceAll(text, CountPlaceholder, countText)), nil
}

func (s *scope) substitute(node ir.ExprNode) (Value, error) {
	text, err := s.text(node.A)
	if err != nil {
		return Nil(), err
	}
	pair, ok := s.plan.Expr(node.B)
	if !ok || pair.Kind != ir.ExprPair {
		return Nil(), fmt.Errorf("expression %d is not an argument pair", node.B)
	}
	name, err := s.text(pair.A)
	if err != nil {
		return Nil(), err
	}
	value, err := s.text(pair.B)
	if err != nil {
		return Nil(), err
	}
	return String(strings.ReplaceAll(text, "{"+name+"}", value)), nil
}

func (s *scope) formOf(number float64) i18n.Form {
	if s.plural == nil {
		return i18n.FormOther
	}
	return s.plural(number)
}

func (s *scope) messageText(message uint32, form i18n.Form) (string, bool) {
	if s.catalog == nil {
		return "", false
	}
	return s.catalog.Text(message, int(form))
}

func formatCount(value Value, number float64, numeric bool) string {
	switch {
	case !numeric:
		return value.Text()
	case value.IsInt():
		return strconv.FormatInt(value.Int(), 10)
	default:
		return strconv.FormatFloat(number, 'f', -1, 64)
	}
}
