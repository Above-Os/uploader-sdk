package cmd

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

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

	fmt.Printf("[Cmd] %s\n", c.cmd.String())
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

// ! feiqi
func (c *Command) RunEx() (string, error) {
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

	stderr, err := c.cmd.StderrPipe()
	if err != nil {
		return "", errors.Wrap(err, "stderr pipe error")
	}

	fmt.Printf("\n>>>> Running command: %s\n\n", c.cmd.String())
	if err := c.cmd.Start(); err != nil {
		return "", errors.Wrap(err, "cmd start error")
	}

	defer close(c.Ch)

	var stdoutBuffer, stderrBuffer bytes.Buffer
	stdoutDone := make(chan error)
	go func() {
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			select {
			case <-c.ctx.Done():
				stdoutDone <- nil
				return
			default:
				line := scanner.Bytes()
				c.Ch <- line
				stdoutBuffer.WriteString(string(line))
				l := strings.TrimSpace(string(line))
				if l == "" {
					return
				}
				if c.options.Print {
					fmt.Println(l)
				}
			}
		}
		stdoutDone <- scanner.Err()
	}()

	stderrDone := make(chan error)
	go func() {
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			select {
			case <-c.ctx.Done():
				stderrDone <- nil
				return
			default:
				line := scanner.Bytes()
				l := strings.TrimSpace(string(line))
				stderrBuffer.WriteString(l)
				if l != "" {
					break
				}
			}
		}
		stderrDone <- scanner.Err()
		return
	}()

	if err := <-stdoutDone; err != nil {
		return "", errors.Wrap(err, "stdout scanner error")
	}
	if err := <-stderrDone; err != nil {
		return "", errors.Wrap(err, "stderr scanner error")
	}
	c.cmd.Wait()

	stdoutResult := stdoutBuffer.String()
	stderrResult := stderrBuffer.String()

	if stderrResult != "" {
		return stdoutResult, errors.New(stderrResult)
	}
	return stdoutResult, nil
}
