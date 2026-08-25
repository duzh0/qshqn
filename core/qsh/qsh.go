package qsh

import (
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/chzyer/readline"
)

const (
	RESET   = "\x1b[0m"
	BOLD    = "\x1b[1m"
	RED     = "\x1b[31m"
	GREEN   = "\x1b[32m"
	YELLOW  = "\x1b[33m"
	BLUE    = "\x1b[34m"
	MAGENTA = "\x1b[35m"
	CYAN    = "\x1b[36m"
	WHITE   = "\x1b[37m"

	PROMPT     = "> "
	CLEAR_LINE = "\r\033[K"

	STRING_GROW_ADD = 50
)

var (
	defaultTerminal = &Terminal{debug: true}
	t               = defaultTerminal

	DEBUG = newMsg("DEBUG", MAGENTA)
	INFO  = newMsg("INFO", WHITE)
	WARN  = newMsg("WARN", YELLOW)
	ERROR = newMsg("ERROR", RED)
	CMD   = newMsg("CMD", BLUE)
	ASK   = newMsg("ASK", CYAN)
)

type Msg struct {
	Head  string
	Color string
}

type Terminal struct {
	debug bool
	mu    sync.Mutex
	rl    *readline.Instance

	askChan chan string
	askmu   sync.Mutex
}

func buildMsg(m *Msg, text string) string {
	var sb strings.Builder
	textLen := len(text)

	sb.Grow(textLen + STRING_GROW_ADD)

	sb.WriteString(BOLD)
	sb.WriteString(m.Color)
	sb.WriteByte('[')
	sb.WriteString(time.Now().Format("02.01.2006 15:04:05"))
	sb.WriteString("] ")
	sb.WriteString(RESET)

	sb.WriteString(BOLD)
	// sb.WriteString(m.Color)
	// sb.WriteByte('[')
	// sb.WriteString(m.Head)
	// sb.WriteByte(']')
	// sb.WriteString(RESET)

	// sb.WriteByte(' ')
	sb.WriteString(text)

	if textLen == 0 || text[textLen-1] != '\n' {
		sb.WriteByte('\n')
	}

	return sb.String()
}

func newMsg(head, color string) *Msg {
	return &Msg{Head: head, Color: color}
}

func write(msg string) {
	if t.rl != nil {
		fmt.Fprint(t.rl, msg)
	} else {
		fmt.Print(msg)
	}
}

func out(m *Msg, text string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	write(buildMsg(m, text))
}

func IsDebug() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.debug
}

func Debug(args ...any) {
	if t.debug {
		out(DEBUG, fmt.Sprint(args...))
	}
}
func Debugf(format string, args ...any) {
	if t.debug {
		out(DEBUG, fmt.Sprintf(format, args...))
	}
}

func Cmd(args ...any)                 { out(CMD, fmt.Sprint(args...)) }
func Cmdf(format string, args ...any) { out(CMD, fmt.Sprintf(format, args...)) }

func Info(args ...any)                 { out(INFO, fmt.Sprint(args...)) }
func Infof(format string, args ...any) { out(INFO, fmt.Sprintf(format, args...)) }

func Warn(args ...any)                 { out(WARN, fmt.Sprint(args...)) }
func Warnf(format string, args ...any) { out(WARN, fmt.Sprintf(format, args...)) }

func Error(args ...any)                 { out(ERROR, fmt.Sprint(args...)) }
func Errorf(format string, args ...any) { out(ERROR, fmt.Errorf(format, args...).Error()) }

func Ask(prompt string) (string, error) {
	t.askmu.Lock()
	defer t.askmu.Unlock()

	t.mu.Lock()
	if t.rl == nil {
		t.mu.Unlock()
		return "", fmt.Errorf("shell not started")
	}

	respChan := make(chan string)
	t.askChan = respChan

	write(buildMsg(ASK, prompt))

	t.mu.Unlock()

	answer, ok := <-respChan
	if !ok {
		return "", fmt.Errorf("shell terminated while waiting for input")
	}

	return answer, nil
}
func Askf(format string, args ...any) (string, error) { return Ask(fmt.Sprintf(format, args...)) }

func Confirm(prompt string, yes []string, no []string, matchCase bool) (bool, error) {
	answer, err := Ask(prompt)
	if err != nil {
		return false, err
	}

	var y, n []string
	if !matchCase {
		answer = strings.ToUpper(answer)
		y = make([]string, len(yes))
		n = make([]string, len(no))
		for i, val := range yes {
			y[i] = strings.ToUpper(val)
		}
		for i, val := range no {
			n[i] = strings.ToUpper(val)
		}
	} else {
		y, n = yes, no
	}

	for {
		switch {
		case slices.Contains(y, answer):
			return true, nil
		case slices.Contains(n, answer):
			return false, nil
		default:
			answer, err = Ask(fmt.Sprintf("enter [%s] or [%s]: ", strings.Join(yes, "/"), strings.Join(no, "/")))
			if err != nil {
				return false, err
			}

			if !matchCase {
				answer = strings.ToUpper(answer)
			}
		}
	}
}

func YesNo(args ...any) (bool, error) {
	return Confirm(fmt.Sprint(append(args, " (y/n): ")...), []string{"y", "yes"}, []string{"n", "no"}, false)
}
func YesNof(prompt string, args ...any) (bool, error) {
	return Confirm(fmt.Sprintf(prompt, args...)+" (y/n): ", []string{"y", "yes"}, []string{"n", "no"}, false)
}

func StartShell() (<-chan string, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.rl != nil {
		return nil, fmt.Errorf("shell already started")
	}

	rl, err := readline.NewEx(&readline.Config{
		Prompt:          PROMPT,
		HistoryFile:     "/tmp/ccai-history",
		InterruptPrompt: "^C",
		EOFPrompt:       "exit",
	})

	if err != nil {
		return nil, err
	}

	t.rl = rl
	cmdChan := make(chan string)

	go func() {
		defer func() {
			t.mu.Lock()
			t.rl = nil

			if t.askChan != nil {
				close(t.askChan)
				t.askChan = nil
			}
			t.mu.Unlock()

			close(cmdChan)
		}()

		for {
			line, err := rl.Readline()
			if err != nil {
				break
			}

			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}

			t.mu.Lock()
			if t.askChan != nil {
				t.askChan <- line
				t.askChan = nil

				t.rl.SetPrompt(PROMPT)
				t.rl.Refresh()
				t.mu.Unlock()
			} else {
				t.mu.Unlock()
				select {
				case cmdChan <- line:
				default:
					Warn("command channel full, dropping command")
				}
			}
		}
	}()

	return cmdChan, nil
}

func IsShellStarted() bool {
	return t.rl != nil
}

func Close() error {
	t.mu.Lock()
	rl := t.rl
	t.rl = nil
	t.mu.Unlock()

	if rl != nil {
		fmt.Print(CLEAR_LINE)
		Debug("shutting down terminal")

		if rl.Config != nil && rl.Config.FuncExitRaw != nil {
			_ = rl.Config.FuncExitRaw()
		}

		go rl.Close()
	}

	return nil
}

func Init(debug bool) error {
	if t != defaultTerminal {
		return fmt.Errorf("tried to init terminal after already initialized")
	}

	defaultTerminal = nil
	t = &Terminal{debug: debug}
	Debug("terminal initialized")

	return nil
}
