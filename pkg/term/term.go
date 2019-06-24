package term

import (
	"errors"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/ricoberger/kubetop/pkg/api"
	"github.com/ricoberger/kubetop/pkg/term/widgets"

	ui "github.com/gizak/termui/v3"
)

// Term represents the user interface for kubetop.
// To initialize the view we need an API client for the interaction with the Kubernetes API.
// We also need a view type to know which view/widget should be rendered.
type Term struct {
	APIClient *api.Client
	ViewType  widgets.ViewType
}

var (
	// ErrInitializeView is thrown if the view could not initialized.
	ErrInitializeView = errors.New("could not initialize view")
)

// Run initialize the user interface and handles the core logic for user interactions.
func (t *Term) Run(filter api.Filter) error {
	// Initialize termui.
	if err := ui.Init(); err != nil {
		return err
	}
	defer ui.Close()

	// Create the view for kubetop.
	// The listActive var to change the behaviour of the key events when the list is shown.
	// We Check ViewType of the term to know which view should be rendered.
	// Then we create the corresponding widget and pass the needed data to this widget (e.g. the width and height of the terminal).
	var listActive bool
	var listType widgets.ListType = widgets.ListTypeSort
	var view widgets.View
	var sortorder api.Sort
	termWidth, termHeight := ui.TerminalDimensions()

	if t.ViewType == widgets.ViewTypeNodes {
		sortorder = api.SortName
		view = widgets.NewNodesWidget(t.APIClient, filter, sortorder, termWidth, termHeight)
	} else if t.ViewType == widgets.ViewTypePods {
		sortorder = api.SortNamespace
		view = widgets.NewPodsWidget(t.APIClient, filter, sortorder, termWidth, termHeight)
	}

	if view == nil {
		return ErrInitializeView
	}

	statusbar := widgets.NewStatusbarWidget(t.APIClient, filter, view.Pause(), sortorder, t.ViewType, termWidth, termHeight)
	list := widgets.NewListWidget(t.APIClient)

	// Create a goroutine for our view to refresh the data every two seconds.
	go func() {
		for {
			view.Update()
			ui.Clear()
			ui.Render(view, statusbar, list)
			time.Sleep(2 * time.Second)
		}
	}()

	// Render our view and get all key events from the user.
	ui.Render(view, statusbar, list)
	uiEvents := ui.PollEvents()

	// Handle kill signal sent event.
	sigTerm := make(chan os.Signal, 2)
	signal.Notify(sigTerm, os.Interrupt, syscall.SIGTERM)

	// Listen for key events or the kill signal event.
	// If we receive the kill signal we exit kubetop.
	// If we receive an key event we handles a corresponding user interaction.
	for {
		select {
		case <-sigTerm:
			return nil
		case e := <-uiEvents:
			switch e.ID {
			case "q", "<C-c>":
				return nil
			case "<Resize>":
				payload := e.Payload.(ui.Resize)
				termWidth, termHeight = payload.Width, payload.Height
				view.SetRect(0, 0, termWidth, termHeight)
				statusbar.SetRect(0, termHeight-1, termWidth, termHeight)
				ui.Clear()
				ui.Render(view, statusbar, list)
			case "k", "<Up>", "<MouseWheelUp>":
				if listActive {
					list.ScrollUp()
					ui.Clear()
					ui.Render(view, statusbar, list)
				} else {
					view.SelectPrev()
					ui.Clear()
					ui.Render(view, statusbar, list)
				}
			case "j", "<Down>", "<MouseWheelDown>":
				if listActive {
					list.ScrollDown()
					ui.Clear()
					ui.Render(view, statusbar, list)
				} else {
					view.SelectNext()
					ui.Clear()
					ui.Render(view, statusbar, list)
				}
			case "<Tab>":
				if t.ViewType == widgets.ViewTypePodDetails {
					view.TabNext()
					view.Update()
					ui.Clear()
					ui.Render(view, statusbar, list)
				}
			case "p":
				view.TogglePause()
				statusbar.SetPause(view.Pause())
				view.Update()
				ui.Clear()
				ui.Render(view, statusbar, list)
			case "<Enter>":
				if listActive {
					sortorder, filter := list.Selected(t.ViewType, listType, view.Sortorder(), view.Filter())
					view.SetSortAndFilter(sortorder, filter)
					statusbar.SetSortAndFilter(sortorder, filter)
					listActive = false
				} else {
					if t.ViewType == widgets.ViewTypeNodes {
						selectedRow := view.SelectedValues()
						nodeFilter := view.Filter()
						nodeFilter.Node = selectedRow[0]

						view = widgets.NewPodsWidget(t.APIClient, nodeFilter, api.SortNamespace, termWidth, termHeight)
						t.ViewType = widgets.ViewTypePods
						statusbar.SetViewType(t.ViewType)
						statusbar.SetPause(false)
					} else if t.ViewType == widgets.ViewTypePods {
						selectedRow := view.SelectedValues()

						view = widgets.NewPodDetailsWidget(selectedRow[1], selectedRow[0], t.APIClient, view.Filter(), view.Sortorder(), termWidth, termHeight)
						t.ViewType = widgets.ViewTypePodDetails
						statusbar.SetViewType(t.ViewType)
						statusbar.SetPause(false)
					}
				}

				view.Update()
				ui.Clear()
				ui.Render(view, statusbar, list)
			case "<Escape>":
				if listActive {
					list.Hide()
					listActive = false
				}

				if t.ViewType == widgets.ViewTypePodDetails {
					view = widgets.NewPodsWidget(t.APIClient, view.Filter(), view.Sortorder(), termWidth, termHeight)
					t.ViewType = widgets.ViewTypePods
					statusbar.SetViewType(t.ViewType)
					statusbar.SetPause(false)
				}

				view.Update()
				ui.Clear()
				ui.Render(view, statusbar, list)
			case "<F1>":
				listType = widgets.ListTypeSort
				listActive = list.Show(t.ViewType, listType, termWidth, termHeight)
				ui.Clear()
				ui.Render(view, statusbar, list)
			case "<F2>":
				listType = widgets.ListTypeFilterNamespace
				listActive = list.Show(t.ViewType, listType, termWidth, termHeight)
				ui.Clear()
				ui.Render(view, statusbar, list)
			case "<F3>":
				listType = widgets.ListTypeFilterNode
				listActive = list.Show(t.ViewType, listType, termWidth, termHeight)
				ui.Clear()
				ui.Render(view, statusbar, list)
			case "<F4>":
				listType = widgets.ListTypeFilterStatus
				listActive = list.Show(t.ViewType, listType, termWidth, termHeight)
				ui.Clear()
				ui.Render(view, statusbar, list)
			}
		}
	}
}
