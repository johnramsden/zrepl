package status

import (
	"fmt"
	"regexp"
	"sync"
	"time"

	"github.com/gdamore/tcell"
	"github.com/pkg/errors"
	"github.com/rivo/tview"

	"github.com/zrepl/zrepl/client/status.v2/client"
	"github.com/zrepl/zrepl/client/status.v2/viewmodel"
	"github.com/zrepl/zrepl/config"
	"github.com/zrepl/zrepl/daemon"
)

type Client interface {
	Status() (daemon.Status, error)
	SignalWakeup(job string) error
	SignalReset(job string) error
}

func Main(config *config.Config, args []string) error {

	// TODO look into https://gitlab.com/tslocum/cview/blob/master/FORK.md

	var err error
	var c Client

	c, err = client.New("unix", config.Global.Control.SockPath)
	if err != nil {
		return errors.Wrapf(err, "connect to daemon socket at %q", config.Global.Control.SockPath)
	}

	app := tview.NewApplication()

	jobDetailSplit := tview.NewFlex()
	jobMenu := tview.NewTreeView()
	jobMenuRoot := tview.NewTreeNode("jobs")
	jobMenuRoot.SetSelectable(true)
	jobMenu.SetRoot(jobMenuRoot).SetCurrentNode(jobMenuRoot)
	jobTextDetail := tview.NewTextView().SetWrap(false)

	jobMenu.SetBorder(true)
	jobTextDetail.SetBorder(true)

	toolbarSplit := tview.NewFlex().SetDirection(tview.FlexRow)
	inputBarContainer := tview.NewFlex()
	fsFilterInput := tview.NewInputField()
	fsFilterInput.SetBorder(false)
	inputBarContainer.AddItem(tview.NewTextView().SetText("[::b]FILTER ").SetDynamicColors(true), 7, 1, false)
	inputBarContainer.AddItem(fsFilterInput, 0, 10, false)
	toolbarSplit.AddItem(inputBarContainer, 1, 0, false)
	toolbarSplit.AddItem(jobDetailSplit, 0, 10, false)

	bottombar := tview.NewFlex().SetDirection(tview.FlexColumn)
	bottombarDateView := tview.NewTextView()
	bottombar.AddItem(bottombarDateView, len(time.Now().String()), 0, false)
	bottomBarStatus := tview.NewTextView().SetDynamicColors(true).SetTextAlign(tview.AlignRight)
	bottombar.AddItem(bottomBarStatus, 0, 10, false)
	toolbarSplit.AddItem(bottombar, 1, 0, false)

	tabbableWithJobMenu := []tview.Primitive{jobMenu, jobTextDetail, fsFilterInput}
	tabbableWithoutJobMenu := []tview.Primitive{jobTextDetail, fsFilterInput}
	var tabbable []tview.Primitive
	tabbableActiveIndex := 0
	tabbableRedraw := func() {
		if len(tabbable) == 0 {
			app.SetFocus(nil)
			return
		}
		if tabbableActiveIndex >= len(tabbable) {
			app.SetFocus(tabbable[0])
			return
		}
		app.SetFocus(tabbable[tabbableActiveIndex])
	}
	tabbableCycle := func() {
		if len(tabbable) == 0 {
			return
		}
		tabbableActiveIndex = (tabbableActiveIndex + 1) % len(tabbable)
		app.SetFocus(tabbable[tabbableActiveIndex])
		tabbableRedraw()
	}

	jobMenuVisisble := false
	reconfigureJobDetailSplit := func(setJobMenuVisible bool) {
		if jobMenuVisisble == setJobMenuVisible {
			return
		}
		jobMenuVisisble = setJobMenuVisible
		if setJobMenuVisible {
			jobDetailSplit.RemoveItem(jobTextDetail)
			jobDetailSplit.AddItem(jobMenu, 0, 1, true)
			jobDetailSplit.AddItem(jobTextDetail, 0, 8, false)
			tabbable = tabbableWithJobMenu
		} else {
			jobDetailSplit.RemoveItem(jobMenu)
			tabbable = tabbableWithoutJobMenu
		}
		tabbableRedraw()
	}

	showModal := func(m *tview.Modal, modalDoneFunc func(idx int, label string)) {
		preModalFocus := app.GetFocus()
		m.SetDoneFunc(func(idx int, label string) {
			if modalDoneFunc != nil {
				modalDoneFunc(idx, label)
			}
			app.SetRoot(toolbarSplit, true)
			app.SetFocus(preModalFocus)
			app.Draw()
		})
		app.SetRoot(m, true)
		app.Draw()
	}

	app.SetRoot(toolbarSplit, true)
	// initial focus
	tabbableActiveIndex = len(tabbable)
	tabbableCycle()
	reconfigureJobDetailSplit(true)

	m := viewmodel.New()
	params := &viewmodel.Params{
		Report:                  nil,
		SelectedJob:             nil,
		FSFilter:                func(_ string) bool { return true },
		DetailViewWidth:         100,
		DetailViewWrap:          false,
		ShortKeybindingOverview: "[::b]<TAB>[::-] switch panes  [::b]Shift+M[::-] toggle navbar  [::b]Shift+S[::-] signal job [::b]</>[::-] filter filesystems",
	}
	paramsMtx := &sync.Mutex{}
	var redraw func()
	viewmodelupdate := func(cb func(*viewmodel.Params)) {
		paramsMtx.Lock()
		defer paramsMtx.Unlock()
		cb(params)
		m.Update(*params)
	}
	redraw = func() {
		jobs := m.Jobs()
		redrawJobsList := false
		var selectedJobN *tview.TreeNode
		if len(jobMenuRoot.GetChildren()) == len(jobs) {
			for i, jobN := range jobMenuRoot.GetChildren() {
				if jobN.GetReference().(*viewmodel.Job) != jobs[i] {
					redrawJobsList = true
					break
				}
				if jobN.GetReference().(*viewmodel.Job) == m.SelectedJob() {
					selectedJobN = jobN
				}
			}
		} else {
			redrawJobsList = true
		}
		if redrawJobsList {
			selectedJobN = nil
			children := make([]*tview.TreeNode, len(jobs))
			for i := range jobs {
				jobN := tview.NewTreeNode(jobs[i].JobTreeTitle()).
					SetReference(jobs[i]).
					SetSelectable(true)
				children[i] = jobN
				jobN.SetSelectedFunc(func() {
					viewmodelupdate(func(p *viewmodel.Params) {
						p.SelectedJob = jobN.GetReference().(*viewmodel.Job)
					})
				})
				if jobs[i] == m.SelectedJob() {
					selectedJobN = jobN
				}
			}
			jobMenuRoot.SetChildren(children)
		}

		if selectedJobN != nil && jobMenu.GetCurrentNode() != selectedJobN {
			jobMenu.SetCurrentNode(selectedJobN)
		} else if selectedJobN == nil {
			// select something, otherwise selection breaks (likely bug in tview)
			jobMenu.SetCurrentNode(jobMenuRoot)
		}

		if selJ := m.SelectedJob(); selJ != nil {
			jobTextDetail.SetText(selJ.FullDescription())
		} else {
			jobTextDetail.SetText("please select a job")
		}

		bottombardatestring := m.DateString()
		bottombarDateView.SetText(bottombardatestring)
		bottombar.ResizeItem(bottombarDateView, len(bottombardatestring), 0)

		bottomBarStatus.SetText(m.BottomBarStatus())

		app.Draw()

	}

	go func() {
		defer func() {
			if err := recover(); err != nil {
				app.Suspend(func() {
					panic(err)
				})
			}
		}()
		t := time.NewTicker(300 * time.Millisecond)
		for _ = range t.C {
			st, err := c.Status()
			viewmodelupdate(func(p *viewmodel.Params) {
				p.Report = st.Jobs
				p.ReportFetchError = err
			})
			app.QueueUpdateDraw(redraw)
		}
	}()

	jobMenu.SetChangedFunc(func(jobN *tview.TreeNode) {
		viewmodelupdate(func(p *viewmodel.Params) {
			p.SelectedJob, _ = jobN.GetReference().(*viewmodel.Job)
		})
		redraw()
		jobTextDetail.ScrollToBeginning()
	})
	jobMenu.SetSelectedFunc(func(jobN *tview.TreeNode) {
		app.SetFocus(jobTextDetail)
	})

	app.SetBeforeDrawFunc(func(screen tcell.Screen) bool {
		viewmodelupdate(func(p *viewmodel.Params) {
			_, _, p.DetailViewWidth, _ = jobTextDetail.GetInnerRect()
		})
		return false
	})

	app.SetInputCapture(func(e *tcell.EventKey) *tcell.EventKey {
		if e.Key() == tcell.KeyTab {
			tabbableCycle()
			return nil
		}

		if e.Key() == tcell.KeyRune && app.GetFocus() == fsFilterInput {
			return e
		}

		if e.Key() == tcell.KeyRune && e.Rune() == '/' {
			if app.GetFocus() != fsFilterInput {
				app.SetFocus(fsFilterInput)
			}
			return e
		}

		if e.Key() == tcell.KeyRune && e.Rune() == 'M' {
			reconfigureJobDetailSplit(!jobMenuVisisble)
			return nil
		}

		if e.Key() == tcell.KeyRune && e.Rune() == 'S' {
			job, ok := jobMenu.GetCurrentNode().GetReference().(*viewmodel.Job)
			if !ok {
				return nil
			}
			signals := []string{"wakeup", "reset"}
			clientFuncs := []func(job string) error{c.SignalWakeup, c.SignalReset}
			sigMod := tview.NewModal().AddButtons(signals)
			sigMod.SetText(fmt.Sprintf("Send a signal to job %q", job.Name()))
			showModal(sigMod, func(idx int, _ string) {
				go func() {
					err := clientFuncs[idx](job.Name())
					if err != nil {
						app.QueueUpdate(func() {
							me := tview.NewModal().SetText(fmt.Sprintf("signal error: %s", err))
							me.AddButtons([]string{"Close"})
							showModal(me, nil)
						})
					}
				}()
			})
		}

		return e
	})

	fsFilterInput.SetChangedFunc(func(searchterm string) {
		viewmodelupdate(func(p *viewmodel.Params) {
			p.FSFilter = func(fs string) bool {
				r, err := regexp.Compile(searchterm)
				if err != nil {
					return true
				}
				return r.MatchString(fs)
			}
		})
		redraw()
		jobTextDetail.ScrollToBeginning()
	})
	fsFilterInput.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Key() == tcell.KeyEnter {
			app.SetFocus(jobTextDetail)
			return nil
		}
		return event
	})

	jobTextDetail.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Key() == tcell.KeyRune && event.Rune() == 'w' {
			// toggle wrapping
			viewmodelupdate(func(p *viewmodel.Params) {
				p.DetailViewWrap = !p.DetailViewWrap
			})
			return nil
		}
		return event
	})

	return app.Run()
}
