package main

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/gcla/gowid"
	"github.com/gcla/gowid/examples"
	"github.com/gcla/gowid/widgets/columns"
	"github.com/gcla/gowid/widgets/framed"
	"github.com/gcla/gowid/widgets/holder"
	"github.com/gcla/gowid/widgets/list"
	"github.com/gcla/gowid/widgets/palettemap"
	"github.com/gcla/gowid/widgets/selectable"
	"github.com/gcla/gowid/widgets/text"
	"github.com/gcla/gowid/widgets/vpadding"
	"github.com/gdamore/tcell/v2"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/pflag"
	"golang.org/x/term"
)

var (
	walker  *list.SimpleListWalker
	listing []FileItem

	flagDir     string
	flagFileCmd string

	cwd   string
	ogDir string

	changeDir chan bool
)

type handler struct{}

func (h handler) UnhandledInput(app gowid.IApp, ev interface{}) bool {
	if evk, ok := ev.(*tcell.EventKey); ok {
		switch evk.Rune() {
		default:
		}
		if evk.Key() == tcell.KeyCtrlC {
			app.Quit()
		}

		if evk.Key() == tcell.KeyEnter {
			position := walker.Focus().(list.ListPos).ToInt()

			if listing[position].IsDir {
				newDir := listing[position].Name
				err := os.Chdir(newDir)
				if err != nil {
					log.Fatal(err)
				}

				cwd = newDir

				changeDir <- true

				return true
			}

			runFileCmd(listing[position])
		}
	}

	return false
}

func runFileCmd(f FileItem) {
	s := strings.ReplaceAll(flagFileCmd, "%f", f.FullPath)
	split := strings.Split(s, " ")

	cmd := exec.Command(split[0], split[1:]...)

	log.Println(cmd.String())

	err := cmd.Run()
	if err != nil {
		log.Fatal(err)
	}
}

type FileItem struct {
	IsDir        bool
	IsExecutable bool
	Name         string
	FullPath     string
}

func getUpDir(dir string) string {
	err := os.Chdir(dir + "/..")
	if err != nil {
		log.Fatal(err)
	}

	updir, err := os.Getwd()
	if err != nil {
		log.Fatal(err)
	}

	_ = os.Chdir(dir)

	return updir
}

func depthCount(path string) int {
	var c int
	for _, v := range path {
		if v == '/' {
			c++
		}
	}

	return c
}

func getDirListing(dir string) []FileItem {
	listing = []FileItem{}

	upDir := FileItem{
		IsDir:        true,
		Name:         "../",
		FullPath:     getUpDir(dir),
		IsExecutable: false,
	}

	first := true
	second := true

	dirDepth := depthCount(dir) + 1

	err := filepath.WalkDir(dir, func(path string, info os.DirEntry, err error) error {
		var f FileItem

		// Hackkyyyyy
		if first {
			// f.IsDir = true
			// f.IsExecutable = false
			// f.FullPath = path
			// f.Name = "./"
			first = false

			// listing = append(listing, f)

			return nil
		} else if second {
			second = false

			listing = append(listing, upDir)
		}

		if depthCount(path) > dirDepth {
			return nil
		}

		if err != nil {
			if errors.Is(err, os.ErrPermission) {
				return nil
			}

			return err
		}

		i, err := info.Info()
		if err != nil {
			return err
		}

		f.IsDir = info.IsDir()
		f.IsExecutable = i.Mode()&0100 != 0
		f.FullPath = path
		f.Name = info.Name()

		if f.IsDir {
			f.Name = f.Name + "/"
		}

		listing = append(listing, f)

		return nil
	})

	if len(listing) == 0 {
		listing = append(listing, upDir)
	}

	if err != nil {
		log.Println(err)
	}

	return listing
}

var styles = gowid.Palette{
	"dir":           gowid.MakePaletteEntry(gowid.ColorRed, gowid.ColorNone),
	"file":          gowid.MakePaletteEntry(gowid.ColorGreen, gowid.ColorNone),
	"executable":    gowid.MakePaletteEntry(gowid.ColorBlue, gowid.ColorNone),
	"invdir":        gowid.MakePaletteEntry(gowid.ColorBlack, gowid.ColorRed),
	"invfile":       gowid.MakePaletteEntry(gowid.ColorBlack, gowid.ColorGreen),
	"invexecutable": gowid.MakePaletteEntry(gowid.ColorBlack, gowid.ColorBlue),
}

func getStyle(f FileItem) string {
	switch {
	case f.IsDir:
		return "invdir"
	case f.IsExecutable:
		return "invexecutable"
	default:
		return "invfile"
	}
}

func getPaletteMap(f FileItem) palettemap.Map {
	switch {
	case f.IsDir:
		return palettemap.Map{"invdir": "dir"}
	case f.IsExecutable:
		return palettemap.Map{"invexecutable": "executable"}
	default:
		return palettemap.Map{"invfile": "file"}
	}
}

func colsUpdate() *columns.Widget {
	w, _, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil {
		log.Fatal(err)
	}

	cwd, err := os.Getwd()
	if err != nil {
		log.Fatal(err)
	}

	if flagDir != "" {
		err = os.Chdir(flagDir)
		if err != nil {
			log.Fatal(err)
		}
		cwd = flagDir
		ogDir = flagDir

		// clear it out so we don't cd back in to it later
		flagDir = ""
	}

	listing := getDirListing(cwd)

	wid := gowid.RenderWithUnits{U: w}

	nl := gowid.MakePaletteRef

	selWidgets := make([]gowid.IWidget, 0)
	for _, v := range listing {
		tCtx := text.NewContent([]text.ContentSegment{
			text.StyledContent(v.Name, nl(getStyle(v))),
		})
		tWid := text.NewFromContent(tCtx)
		wSel := selectable.New(palettemap.New(tWid, palettemap.Map{}, getPaletteMap(v)))

		selWidgets = append(selWidgets, wSel)
	}

	walker = list.NewSimpleListWalker(selWidgets)
	listingWidget := list.New(walker)
	listbox := vpadding.NewBox(listingWidget, 7)
	frame := framed.New(listbox, framed.Options{
		Frame: framed.FrameRunes{' ', ' ', ' ', ' ', ' ', ' ', ' ', ' '},
		Title: cwd,
	})

	cols := columns.New([]gowid.IContainerWidget{
		&gowid.ContainerWidget{
			IWidget: frame,
			D:       wid,
		},
	})
	return cols
}

func main() {
	cols := colsUpdate()
	h := holder.New(cols)

	args := gowid.AppArgs{
		View:    h,
		Palette: &styles,
		Log:     log.StandardLogger(),
	}

	changeDir = make(chan bool, 1)

	app, err := gowid.NewApp(args)

	go func() {
		for {
			<-changeDir

			err = app.Run(gowid.RunFunction(func(app gowid.IApp) {
				cols = colsUpdate()
				h.SetSubWidget(cols, app)
			}))

			if err != nil {
				log.Println(err)
			}
		}

	}()

	examples.ExitOnErr(err)

	f := examples.RedirectLogger("out.log")
	defer f.Close()

	app.MainLoop(handler{})
}

func init() {
	pflag.StringVarP(&flagDir, "dir", "d", "", "dir")
	pflag.StringVarP(&flagFileCmd, "cmd", "c", "", "cmd to run on file open (replace %f with file (full)path)")
	pflag.Parse()
}
