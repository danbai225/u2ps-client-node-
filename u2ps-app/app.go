package main

import (
	"fyne.io/fyne/app"
	"fyne.io/fyne/widget"
	"os"
	"u2ps/client"
	"u2ps/common"
)
type enterEntry struct {
	widget.Entry
}

func newEnterEntry() *enterEntry {
	entry := &enterEntry{}
	entry.ExtendBaseWidget(entry)
	return entry
}
func main() {
	//converts a  string from UTF-8 to gbk encoding.
	a := app.New()
	w := a.NewWindow("U2PS")
	entry := newEnterEntry()
	state := widget.NewLabel("Status:No Run")
	bt := widget.NewButton("Run", func() {
		if len(entry.Text)>=1{
			state.SetText("Status:Run")
			entry.Hidden = true
			common.Key = entry.Text
			entry.Text=""
			common.HostInfo = "server.u2ps.com:2251"
			common.MaxRi = 10
			go client.Conn()
		}
	})
	w.SetContent(widget.NewVBox(
		widget.NewLabel("Versions:"+common.Versions),
		widget.NewLabel("Please enter the key"),
		entry,
		state,
		bt,
		widget.NewButton("Exit", func() {
			os.Exit(1)
		})))
	w.ShowAndRun()
}

