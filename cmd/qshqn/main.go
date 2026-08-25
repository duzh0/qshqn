package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"qshqn/core/ai"
	"qshqn/core/command"
	"qshqn/core/config"
	"qshqn/core/db"
	"qshqn/core/fiox"
	"qshqn/core/locale"
	"qshqn/core/qsh"
	"qshqn/core/tgx"
)

func createExampleInitFile() error {
	initFile := config.GetExampleInitFile()
	path := config.EXAMPLE_INIT_FILE_PATH
	if err := fiox.Save(path, initFile, fiox.CreateOnly, fiox.NoSetCache); err != nil {
		return fmt.Errorf("error creating example config file at [%s]: %w", path, err)
	}

	qsh.Infof("created init file at [%s]", path)
	return nil
}

func regenInitFile() error {
	if regen, err := qsh.YesNo("regenerate clean example file?"); err != nil {
		return fmt.Errorf("error confirming regeneration: %w", err)
	} else if regen {
		if err := createExampleInitFile(); err != nil {
			return err
		}

		return fmt.Errorf("example config created at [%s]. edit name to [%s], fill values in and restart", config.EXAMPLE_INIT_FILE_PATH, config.INIT_FILE_PATH)
	}

	return fmt.Errorf("init file is invalid and regeneration was declined")
}

func loadInitFile() (config.InitFile, error) {
	initFile, err := fiox.Load[config.InitFile](config.INIT_FILE_PATH, fiox.NoReadCache, fiox.NoSetCache)
	if err == nil {
		if valErr := initFile.Validate(); valErr != nil {
			qsh.Errorf("invalid init file: %w", valErr)
			return initFile, regenInitFile()
		}
		return initFile, nil
	}

	if os.IsNotExist(err) || errors.Is(err, os.ErrNotExist) {
		if create, err := qsh.YesNof("init file at [%s] not found. create example file?", config.INIT_FILE_PATH); err != nil {
			return initFile, fmt.Errorf("error confirming creation: %w", err)
		} else if create {
			if err := createExampleInitFile(); err != nil {
				return initFile, err
			}
			return initFile, fmt.Errorf("missing init file. example created at [%s], please edit and restart", config.EXAMPLE_INIT_FILE_PATH)
		}

		return initFile, fmt.Errorf("missing init file")
	}

	if syntaxErr, ok := errors.AsType[*json.SyntaxError](err); ok {
		qsh.Errorf("json syntax error in [%s] at offset %d", config.INIT_FILE_PATH, syntaxErr.Offset)
		return initFile, err
	}

	if typeErr, ok := errors.AsType[*json.UnmarshalTypeError](err); ok {
		qsh.Errorf("type mismatch in [%s]: field [%s] expects %v but got %v", config.INIT_FILE_PATH, typeErr.Field, typeErr.Type, typeErr.Value)
		return initFile, regenInitFile()
	}

	return initFile, err
}

func run() error {
	cmds, err := qsh.StartShell()
	if err != nil {
		return fmt.Errorf("error starting shell: %w", err)
	}

	qsh.Debug("shell started")

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	go func() {
		defer cancel()
		for cmd := range cmds {
			cmdUpper := strings.ToUpper(cmd)
			if cmdUpper == "EXIT" {
				return
			}
			qsh.Cmdf("you said: %s", cmd)
		}
	}()

	initFile, err := loadInitFile()
	if err != nil {
		return fmt.Errorf("error loading init file: %w", err)
	}

	config.Init(initFile)

	if err = locale.Init(locale.InitOptions{}); err != nil {
		return fmt.Errorf("error initializing locale module: %w", err)
	}

	if err = command.Init(); err != nil {
		return fmt.Errorf("error initializing command module: %w", err)
	}

	var newPath string
	if newPath, err = db.Init(
		&db.InitOptions{
			DBPath: initFile.Db.Path,
			PredatorMsgs: db.PredatorMsgsInit{
				Path:       initFile.Predator.Msgs.Path,
				ImportMode: initFile.Predator.Msgs.ImportMode,
			},
		},
	); err != nil {
		return fmt.Errorf("error initializing db module: %w", err)
	}

	if newPath != config.Db.Path {
		qsh.Debugf("updating db path from [%s] to [%s]", config.Db.Path, newPath)
		initFile.Db.Path = newPath
		if err = fiox.Save(config.INIT_FILE_PATH, initFile, fiox.UpdateOnly, fiox.NoSetCache); err != nil {
			return fmt.Errorf("error updating db path in initFile[%s]: %w", config.INIT_FILE_PATH, err)
		}
	}

	if err = ai.Init(); err != nil {
		return fmt.Errorf("error initializing ai module: %w", err)
	}

	botToken := initFile.Tg.Creds.BotToken
	if initFile.Tg.UseTestToken {
		qsh.Debugf("using test bot token")
		botToken = initFile.Tg.Creds.TestBotToken
	}

	man, err := tgx.New(initFile.Tg.Creds.AppID, initFile.Tg.Creds.AppHash, botToken, initFile.Tg.SessionPath)
	if err != nil {
		return fmt.Errorf("error creating a tgx manager: %w", err)
	}

	if err = man.Run(ctx); err != nil {
		return fmt.Errorf("tgx runtime error: %w", err)
	}

	return nil
}

func main() {
	if err := qsh.Init(true); err != nil {
		fmt.Printf("error initializing console: %v\n", err)
		return
	}

	defer qsh.Close()
	defer db.Close()
	if err := run(); err != nil {
		qsh.Errorf("critical error: %w", err)
	}
}
