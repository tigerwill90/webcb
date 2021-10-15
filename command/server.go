package command

import (
	"github.com/urfave/cli/v2"
)

type server struct {
}

func newServer() *server {
	return &server{}
}

func (s *server) run() cli.ActionFunc {
	return func(cc *cli.Context) error {
		return nil
	}
}
