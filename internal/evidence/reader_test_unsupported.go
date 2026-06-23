//go:build !linux

package evidence

import "errors"

func makeFIFO(path string) error { return errors.New("fifo test unsupported") }
