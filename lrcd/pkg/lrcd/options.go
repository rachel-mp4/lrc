package lrcd

import (
	"errors"
	"unicode/utf8"
	"io"
	"time"
)

func WithTCPPort(port int) Option {
	return func(options *options) error {
		if port < 0 {
			return errors.New("port should be postive")
		}
		options.portTCP = &port
		return nil
	}
}

func WithWSPort(port int) Option {
	return func(options *options) error {
		if port < 0 {
			return errors.New("port should be postive")
		}
		options.portWS = &port
		return nil
	}
}

func WithWSPath(path string) Option {
	return func(options *options) error {
		options.pathWS = &path
		return nil
	}
}

func WithWelcome(welcome string) Option {
	return func(options *options) error {
		if utf8.RuneCountInString(welcome) > 50 {
			return errors.New("welcome must be at most 50 runes")
		}
		options.welcome = &welcome
		return nil
	}
}

func WithLogging(w io.Writer, verbose bool) Option {
	return func(options *options) error {
		if w == nil {
			return errors.New("must provide a writer to log to")
		}
		options.writer = &w
		options.verbose = verbose
		return nil
	}
}

func WithEmptyChannel(emptyChan chan struct{}) Option {
	return func(options *options) error {
		if emptyChan == nil {
			return errors.New("must provide a channel to signal on")
		}
		options.emptyChan = emptyChan
		return nil
	}
}

func WithEmptySignalAfter(after time.Duration) Option {
	return func(options *options) error {
		if after < 0*time.Second {
			return errors.New("after must be positive")
		}
		options.timeToEmit = &after
		return nil
	}
}