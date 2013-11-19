package main

import (
	"fmt"
	"os"
	"os/exec"
	"io/ioutil"
	"path/filepath"
	"time"
	"strings"
	"github.com/howeyc/fsnotify"
	"github.com/jessevdk/go-flags"
	"github.com/shxsun/klog"
	"encoding/json"
)

var (
	K           = klog.NewLogger(nil, "")
	notifyDelay time.Duration
	LeftRight   = strings.Repeat("-", 10)

	// golang c/cpp php js
	LangExts = []string{".go", ".cpp", ".c", ".h", ".php", ".js"}
)

// Add dir and children (recursively) to watcher
func watchDirAndChildren(w *fsnotify.Watcher, path string, depth int) error {
	if err := w.Watch(path); err != nil {
		return err
	}
	baseNumSeps := strings.Count(path, string(os.PathSeparator))
	return filepath.Walk(path, func(path string, info os.FileInfo, err error) error {
		if info.IsDir() {
			pathDepth := strings.Count(path, string(os.PathSeparator)) - baseNumSeps
			if pathDepth > depth {
				return filepath.SkipDir
			}
			if opts.Verbose {
				fmt.Fprintln(os.Stderr, "Watching", path)
			}
			if err := w.Watch(path); err != nil {
				return err
			}
		}
		return nil
	})
}

// generate new event
func NewEvent(paths []string, depth int) chan *fsnotify.FileEvent {

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		K.Fatalf("fail to create new Watcher: %s", err)
	}
	K.Info("Initial watcher")

	for _, path := range paths {
		K.Debugf("watch directory: %s", path)
		err = watchDirAndChildren(watcher, path, depth)
		if err != nil {
			K.Fatal("fail to watch directory: %s", err)
		}
	}

	// ignore watcher error
	go func() {
		for {
			err := <-watcher.Error          // ignore watcher error
			K.Warnf("watch error: %s", err) // No need to exit here
		}
	}()

	return watcher.Event
}

func execute(e chan *fsnotify.FileEvent, origCmd *exec.Cmd) {
	var cmd *exec.Cmd
	filterEvent := filter(e)
	// make first time, do command
	go func() {
		filterEvent <- &fsnotify.FileEvent{}
	}()
	for {
		ev := <-filterEvent
		K.Info("Sense first:", ev)
	CHECK:
		select {
		case ev = <-filterEvent:
			K.Info("Sense again: ", ev)
			goto CHECK
		case <-time.After(notifyDelay):
		}
		// stop cmd
		if cmd != nil && cmd.Process != nil {
			K.Info("stop process")
			cmd.Process.Kill()
		}
		// create new cmd
		newCmd := *origCmd
		cmd = &newCmd
		K.Info(fmt.Sprintf("%s %5s %s", LeftRight, "START", LeftRight))
		err := cmd.Start()
		if err != nil {
			K.Error(err)
			continue
		} else {
			go func(cmd *exec.Cmd) {
				err := cmd.Wait()
				if err != nil {
					K.Error(fmt.Sprintf("%s %5s %s", LeftRight, "ERROR", LeftRight))
				} else {
					K.Info(fmt.Sprintf("%s %5s %s", LeftRight, "E N D", LeftRight))
				}
			}(cmd)
		}
	}
}

var opts struct {
	Verbose bool   `short:"v" long:"verbose" description:"Show verbose debug infomation"`
	Delay   string `long:"delay" description:"Trigger event buffer time" default:"0.5s"`
	Depth   int    `short:"d" long:"depth" description:"depth of watch" default:"1"`
}

type watchConfig struct {
	Cmd     string
	Path   []string
}
func main() {
	parser := flags.NewParser(&opts, flags.Default|flags.PassAfterNonOption)
	args, err := parser.Parse()
	if err != nil {
		K.Fatal(err)
		os.Exit(1)
	}
	if opts.Verbose {
		K.SetLevel(klog.LDebug)
	}
	notifyDelay, err = time.ParseDuration(opts.Delay)
	if err != nil {
		K.Warn(err)
		notifyDelay = time.Millisecond * 500
	}

	K.Debugf("delay time: %s", notifyDelay)

	if len(args) == 0 {
		fmt.Printf("Use %s --help for more details\n", os.Args[0])
		return
	}

	// // check if cmd exists
	// _, err = exec.LookPath(args[0])
	// if err != nil {
	// 	fmt.Println(1)
	// }
	dataByte,err := ioutil.ReadFile(args[0])

	if err!=nil{
		K.Fatal(err)
	}

	var config watchConfig

	err = json.Unmarshal(dataByte, &config)
	if err != nil {
		K.Fatal(err)
	}

	cmdSlice :=strings.Split(config.Cmd," ")

	cmd := exec.Command(cmdSlice[0],cmdSlice[1:]...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	event := NewEvent(config.Path, 10)
	execute(event, cmd)
}
