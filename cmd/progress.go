package cmd

import (
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/spf13/cobra"
)

var tick = 100 * time.Millisecond

const frames = "⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏"

func step(c *cobra.Command, doing string) func(err error, done string) {
	out := c.ErrOrStderr()
	if !terminal(out) {
		fmt.Fprintln(out, doing+"…")
		return func(error, string) {}
	}
	spinning := &spinner{out: out, doing: doing, stop: make(chan struct{}), stopped: make(chan struct{})}
	c.SetErr(spinning)
	go spinning.spin()
	color := paletteFor(out)
	return func(err error, done string) {
		close(spinning.stop)
		<-spinning.stopped
		c.SetErr(out)
		if err != nil {
			fmt.Fprintf(out, "\r\x1b[K%s %s\n", color.paint("31", "X"), doing)
			return
		}
		fmt.Fprintf(out, "\r\x1b[K%s %s\n", color.paint("32", "✓"), done)
	}
}

type spinner struct {
	sync.Mutex
	out           io.Writer
	doing         string
	stop, stopped chan struct{}
}

func (s *spinner) Write(p []byte) (int, error) {
	s.Lock()
	defer s.Unlock()
	fmt.Fprint(s.out, "\r\x1b[K")
	return s.out.Write(p)
}

func (s *spinner) spin() {
	defer close(s.stopped)
	ticker := time.NewTicker(tick)
	defer ticker.Stop()
	spinner := []rune(frames)
	for frame := 0; ; frame++ {
		s.Lock()
		fmt.Fprintf(s.out, "\r\x1b[K%c %s…", spinner[frame%len(spinner)], s.doing)
		s.Unlock()
		select {
		case <-s.stop:
			return
		case <-ticker.C:
		}
	}
}
