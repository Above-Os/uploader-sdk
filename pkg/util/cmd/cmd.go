package cmd

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"

	"bytetrade.io/web3os/backups-sdk/pkg/util/logger"
	"github.com/pkg/errors"
)

type Command struct {
	options CommandOptions
	ctx     context.Context
	cancel  context.CancelFunc
	cmd     *exec.Cmd
	Ch      chan []byte
}

type CommandOptions struct {
	Path  string
	Args  []string
	Envs  map[string]string
	Print bool
}

func NewCommand(ctx context.Context, opts CommandOptions) *Command {
	var cmdCtx, cancel = context.WithCancel(context.Background())
	return &Command{
		options: opts,
		ctx:     cmdCtx,
		cancel:  cancel,
		Ch:      make(chan []byte, 50),
	}
}

func (c *Command) Cancel() {
	c.cancel()
}

func (c *Command) GetCmd() *exec.Cmd {
	return c.cmd
}

func (c *Command) Run() (string, error) {
	var result string
	var err error
	c.cmd = exec.CommandContext(c.ctx, c.options.Path, c.options.Args...)
	c.cmd.Env = append(os.Environ(), c.cmd.Env...)

	for k, v := range c.options.Envs {
		c.cmd.Env = append(c.cmd.Env, fmt.Sprintf("%s=%s", k, v))
	}

	stdout, err := c.cmd.StdoutPipe()
	if err != nil {
		return "", errors.Wrap(err, "stdout pipe error")
	}
	c.cmd.Stderr = c.cmd.Stdout

	logger.Infof("[Cmd] %s", c.cmd.String())
	if err := c.cmd.Start(); err != nil {
		return "", errors.Wrap(err, "cmd start error")
	}

	defer func() (string, error) {
		close(c.Ch)
		if errWait := c.cmd.Wait(); errWait != nil {
			return "", errors.Wrapf(errWait, fmt.Sprintf("wait error for command: %s, exec error %v", c.cmd.String(), err))
		}
		return result, err
	}()

	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		select {
		case <-c.ctx.Done():
			return result, nil
		default:
			line := scanner.Bytes()
			c.Ch <- line
		}
	}

	if err := scanner.Err(); err != nil {
		return result, errors.Wrap(err, "scanner error")
	}

	return result, nil
}
