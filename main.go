package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"path"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/fsnotify/fsnotify"
)

type Program struct {
	Name                 string
	WorkSpace            string
	ExcludeDirs          []string
	Watcher              *fsnotify.Watcher
	buildCmdStr          string
	SubProccesPid        int
	SubProccesRunCommand string
	running              chan int
}

var (
	run      string
	debug    bool
	excludes string
	logger   *log.Logger
)

func init() {
	help()
	logger = log.New(os.Stdout, "", log.Ldate|log.Ltime|log.Lshortfile)
}

func main() {
	p := New()
	go p.handleEvent()
	go p.handleSignal()
	p.watchRecursively(p.WorkSpace)
	select {}
}

func help() {
	flag.StringVar(&run, "r", "", "run Program after auto build")
	flag.BoolVar(&debug, "d", false, "run Program in debug mode")
	flag.StringVar(&excludes, "e", ".git", `exclude monitor dir,can list multiple, -e "dir1 dir2 dir3"`)

	old := flag.Usage
	flag.Usage = func() {
		old()
		fmt.Println("  cd workspace,run goreload,this will auto build code and run\n")
	}
	flag.Parse()
}
func (p *Program) watch() {

	myWatcher, e := fsnotify.NewWatcher()
	if e != nil {
		logger.Fatal(e)
	}
	if e := myWatcher.Add(p.WorkSpace); e != nil {
		logger.Fatal(e)
	}
	myWatcher.Add(p.WorkSpace)
	for ev := range myWatcher.Events {
		fmt.Println(ev)
	}

}
func (p *Program) watchRecursively(dirOrFile string) {
	for _, excludeDir := range p.ExcludeDirs {
		if dirOrFile == excludeDir {
			if debug {
				logger.Println("exclude:", excludeDir)
			}
			return
		}

	}

	finfoSlice, e := ioutil.ReadDir(dirOrFile)
	if e != nil {
		if debug {
			logger.Println(e)
		}
		return
	} else {
		if debug {
			fmt.Println("watch target:", dirOrFile)
		}
		p.Watcher.Add(dirOrFile)
		for _, finfo := range finfoSlice {
			if finfo.IsDir() {
				fname := dirOrFile + string(filepath.Separator) + finfo.Name()
				p.watchRecursively(fname)
			}
		}
	}

}
func (p *Program) handleEvent() {

	for {
		select {
		case event, ok := <-p.Watcher.Events:
			if !ok {
				fmt.Println("channel close")
				return
			}
			if event.Op == fsnotify.Remove {
				p.Watcher.Remove(event.Name)
			} else if event.Op == fsnotify.Create {
				p.watchRecursively(event.Name)
			} else if event.Op == fsnotify.Write {
				if debug {
					fmt.Println("receive:", event.String())
				}
				p.handleWrite(event.Name)
				<-p.running
				if debug {
					fmt.Println("receive", event.String(), "end!")
				}
			}
		}
	}
}
func (p *Program) handleWrite(file string) {
	if debug {
		logger.Println("handleWrite event start")
		logger.Println("sub process pid:", p.SubProccesPid)

	}

	if strings.HasSuffix(file, ".go") {
		//如果存在旧的进程，kill掉
		if p.SubProccesPid != 0 {
			if e := syscall.Kill(-p.SubProccesPid, syscall.SIGTERM); e != nil {
				logger.Println(e)
			} else {
				logger.Println("kill pid", p.SubProccesPid)
			}
		}
		if debug {
			fmt.Println("start compile")
		}
		cmd := exec.Command("bash", "-c", p.buildCmdStr)
		output, e := cmd.CombinedOutput()
		if e != nil {
			fmt.Println("build error:", e)
		}
		fmt.Println(string(output))

		if run != "" {
			if debug {
				logger.Println("start run program")
			}
			cmd := exec.Command("bash", "-c", p.SubProccesRunCommand)
			cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			go func() {
				if e := cmd.Start(); e != nil {
					logger.Println(e)
					return
				} else {
					if debug {
						logger.Println("run cmd pid:", cmd.Process.Pid)
					}
					p.SubProccesPid = cmd.Process.Pid
					p.running <- 1
					if e := cmd.Wait(); e != nil {
						logger.Println("run error", e)
					}
					p.SubProccesPid = 0

				}
			}()
		}
	} else {
		p.running <- 1
	}
	if debug {
		logger.Println("handleWrite event done")
	}
}
func (p *Program) isDir(name string) (is bool, err error) {
	is = false
	finfo, err := os.Stat(name)
	is = finfo.IsDir()
	return
}

//New create *Program p
func New() *Program {
	dir, e := os.Getwd()
	if e != nil {
		logger.Fatal(e)
	}
	gomod := dir + string(filepath.Separator) + "go.mod"
	_, e = os.Stat(gomod)
	if e != nil {
		if os.IsNotExist(e) {
			logger.Println(e)
			fmt.Println("the watch directory must have go.mod")
			os.Exit(2)
		}
		logger.Fatal(e)
	}
	baseName := path.Base(dir)

	myWatcher, e := fsnotify.NewWatcher()
	if e != nil {
		logger.Fatal(e)
	}
	eSlice := getExcludeSlice(excludes, dir)
	p := &Program{
		Name:                 baseName,
		WorkSpace:            dir,
		Watcher:              myWatcher,
		ExcludeDirs:          eSlice,
		buildCmdStr:          fmt.Sprintf("go build -o %s *.go", baseName),
		running:              make(chan int, 1),
		SubProccesRunCommand: run,
	}
	return p
}

func getExcludeSlice(excludes string, dir string) (eSlice []string) {
	newExcludes := strings.Trim(excludes, " ")
	tmpSlice := strings.Fields(newExcludes)

	for _, exclude := range tmpSlice {
		exclude = dir + string(filepath.Separator) + exclude
		eSlice = append(eSlice, exclude)
	}
	if debug {
		logger.Println("exclude list:", eSlice)
	}
	return
}
func (p *Program) handleSignal() {
	c := make(chan os.Signal, 1)
	signal.Notify(c, syscall.SIGINT)
	for {
		select {
		case <-c:
			if p.SubProccesPid != 0 {
				if debug {
					logger.Println("kill process", p.SubProccesPid)
					time.Sleep(time.Second * 1)
				}
				syscall.Kill(p.SubProccesPid, syscall.SIGTERM)
			}
			os.Exit(2)
		}
	}
}
