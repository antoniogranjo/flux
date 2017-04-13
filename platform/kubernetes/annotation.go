package kubernetes

import (
	"fmt"
	"io"
)

func applyAnnotation(in string, ann map[string]string, trace, out io.Writer) error {
	return fmt.Errorf("FIXME")
}

func removeAnnotation(in string, annKey string, trace, out io.Writer) error {
	return fmt.Errorf("FIXME")
}
