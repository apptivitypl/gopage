package runtime

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/apptivitypl/gopage/internal/ir"
)

const (
	keySeparator = "\x1f"
	absentMark   = '-'
)

type fragmentSave struct {
	fragment ir.Fragment
	key      string
	start    int
	end      int
}

func (s *scope) openFragment(op ir.Op, out *Buffer, hook Fragments) (bool, fragmentSave, error) {
	fragment, ok := s.plan.Fragment(op.A)
	if !ok {
		return false, fragmentSave{}, fmt.Errorf("plan names fragment %d, which is not in the table", op.A)
	}
	if hook == nil || !fragment.Cacheable() {
		return false, fragmentSave{}, nil
	}
	key, err := s.fragmentKey(fragment)
	if err != nil {
		return false, fragmentSave{}, err
	}
	if body, found := hook.Load(fragment, key); found {
		out.Write(body)
		return true, fragmentSave{}, nil
	}
	return false, fragmentSave{fragment: fragment, key: key, start: out.Len(), end: int(op.B)}, nil
}

func (s *scope) fragmentKey(fragment ir.Fragment) (string, error) {
	if len(fragment.Paths) == 0 {
		return fragment.Name, nil
	}
	var b strings.Builder
	b.WriteString(fragment.Name)
	for _, index := range fragment.Paths {
		path := s.plan.Path(index)
		if path == nil {
			return "", fmt.Errorf("fragment %s reads path %d, which is not in the table", fragment.Name, index)
		}
		b.WriteString(keySeparator)
		value, ok := s.props.Get(path)
		if !ok {
			b.WriteByte(absentMark)
			continue
		}
		text := value.Text()
		b.WriteString(strconv.Itoa(len(text)))
		b.WriteByte(':')
		b.WriteString(text)
	}
	return b.String(), nil
}
