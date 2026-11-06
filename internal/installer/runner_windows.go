package installer

import (
	"context"
	"errors"
	"io"
)

type Runner struct{ Log io.Writer }

func (Runner) Run(context.Context, string) error {
	return errors.New("installer requires Linux")
}
