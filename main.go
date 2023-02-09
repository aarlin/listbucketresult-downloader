package main

import (
	"bufio"
	"context"
	"encoding/csv"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/aarlin/listbucketresult-downloader/client"
	utils "github.com/aarlin/listbucketresult-downloader/utils"
	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var (
	focusedStyle        = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))
	blurredStyle        = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	cursorStyle         = focusedStyle.Copy()
	noStyle             = lipgloss.NewStyle()
	helpStyle           = blurredStyle.Copy()
	cursorModeHelpStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("244"))

	focusedButton = focusedStyle.Copy().Render("[ Submit ]")
	blurredButton = fmt.Sprintf("[ %s ]", blurredStyle.Render("Submit"))

	focusedCursor         = focusedStyle.Copy().Render(">")
	checkedSelectionStyle = focusedStyle.Copy().Render("✓")

	currentUrlStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("211"))
	doneStyle       = lipgloss.NewStyle().Margin(1, 2)
	checkMark       = lipgloss.NewStyle().Foreground(lipgloss.Color("42")).SetString("✓")
	failMark        = lipgloss.NewStyle().Foreground(lipgloss.Color("9")).SetString("X")
)

type statusMsg int
type errMsg struct {
	err error
}

func (e errMsg) Error() string { return e.err.Error() }

type item struct {
	text    string
	checked bool
}

type GotResources struct {
	Err       error
	Resources []string
}

type DownloadResourceResp struct {
	Err   error
	Msg   string
	Index int
}

func (m model) Init() tea.Cmd {
	m.preloadLastInputs()

	return tea.Batch(
		textinput.Blink,
		m.spinner.Tick,
	)
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "esc", "q":
			return m, tea.Quit
		case "tab", "shift+tab", "enter", "up", "down", " ":
			s := msg.String()
			if m.showTypingView {

				// Did the user press enter while the submit button was focused?
				// If so, exit.
				if s == "enter" && m.focusIndex == len(m.inputs) {
					m.showTypingView = false
					m.showLoadingView = true
					m.focusIndex = 0

					m.saveInputs()

					return m, tea.Batch(
						spinner.Tick,
						m.fetchResources(),
					)
				}

				// Cycle indexes
				if s == "up" || s == "shift+tab" {
					m.focusIndex--
				} else {
					m.focusIndex++
				}

				if m.focusIndex > len(m.inputs) {
					m.focusIndex = 0
				} else if m.focusIndex < 0 {
					m.focusIndex = len(m.inputs)
				}

				cmds := make([]tea.Cmd, len(m.inputs))
				for i := 0; i <= len(m.inputs)-1; i++ {
					if i == m.focusIndex {
						// Set focused state
						cmds[i] = m.inputs[i].Focus()
						m.inputs[i].PromptStyle = focusedStyle
						m.inputs[i].TextStyle = focusedStyle
						continue
					}
					// Remove focused state
					m.inputs[i].Blur()
					m.inputs[i].PromptStyle = noStyle
					m.inputs[i].TextStyle = noStyle
				}

				return m, nil
			}

		}
	case GotResources:
		m.showLoadingView = false
		m.showDownloadingView = true

		if err := msg.Err; err != nil {
			m.err = err
			return m, tea.Batch(
				tea.Printf("[GotResources]: error occured: %s\n", err),
			)
		}

		m.resources = msg.Resources

		progressCmd := m.progress.SetPercent(float64(m.downloadCount) / float64(len(m.resources)))

		return m, tea.Batch(
			progressCmd,
			tea.Printf("%s %s", checkMark, m.resources[m.downloadCount]), // print success message above our program
			m.listenForActivity(m.sub),                                   // generate activity
			waitForActivity(m.sub),                                       // wait for activity
		)
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	case progress.FrameMsg:
		newModel, cmd := m.progress.Update(msg)
		if newModel, ok := newModel.(progress.Model); ok {
			m.progress = newModel
		}
		return m, cmd
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
	case DownloadResourceResp:
		m.downloadCount = msg.Index + 1
		if m.downloadCount >= len(m.resources) {
			// Everything's been installed. We're done!
			m.finishedDownloading = true
			return m, tea.Quit
		}

		progressCmd := m.progress.SetPercent(float64(m.downloadCount) / float64(len(m.resources)))

		if err := msg.Err; err != nil {
			return m, tea.Batch(
				progressCmd,
				tea.Printf("%s %s", failMark, m.resources[m.downloadCount]), // print success message above our program
				waitForActivity(m.sub), // wait for activity
			)
		}

		return m, tea.Batch(
			progressCmd,
			tea.Printf("%s %s", checkMark, m.resources[m.downloadCount]), // print success message above our program
			waitForActivity(m.sub), // wait for activity
		)
	}

	cmd := m.updateInputs(msg)

	return m, cmd
}

func (m model) View() string {

	s := ""

	if m.showLoadingView {
		url := m.inputs[0].Value()
		keyOffset := m.inputs[3].Value()
		spinnerText := fmt.Sprintf("Retrieving resource paths from: %s and key offset: %s, please wait ... \n\n", url, keyOffset)

		s += "\n" +
			m.spinner.View() + spinnerText
	}

	if m.showDownloadingView {
		n := len(m.resources)
		w := lipgloss.Width(fmt.Sprintf("%d", n))

		if n == 0 {
			return doneStyle.Render(fmt.Sprintf("There were no resources downloaded.\n"))
		}
		if m.finishedDownloading {
			return doneStyle.Render(fmt.Sprintf("Done! Downloaded %d resources.\n", n - 1))
		}

		resourceCount := fmt.Sprintf(" %*d/%*d", w, m.downloadCount, w, n-1)

		spin := m.spinner.View() + " "
		prog := m.progress.View()
		cellsAvail := utils.Max(0, m.width-lipgloss.Width(spin+prog+resourceCount))

		url := currentUrlStyle.Render(m.resources[m.downloadCount])
		info := lipgloss.NewStyle().MaxWidth(cellsAvail).Render("Downloading " + url)

		cellsRemaining := utils.Max(0, m.width-lipgloss.Width(spin+info+prog+resourceCount))
		gap := strings.Repeat(" ", cellsRemaining)

		s += spin + info + gap + prog + resourceCount
	}

	if m.showTypingView {
		s += "Fill out the following\n\n"

		var b strings.Builder

		for i := range m.inputs {
			b.WriteString(m.inputs[i].View())
			if i < len(m.inputs)-1 {
				b.WriteRune('\n')
			}
		}

		button := &blurredButton
		if m.focusIndex == len(m.inputs) {
			button = &focusedButton
		}
		fmt.Fprintf(&b, "\n\n%s\n\n", *button)

		s += b.String()
	}

	if m.showErrorView {
		s += m.err.Error()
	}

	s += "\nPress q to quit.\n"
	return s
}

func (m *model) listenForActivity(sub chan DownloadResourceResp) tea.Cmd {
	return func() tea.Msg {
		for {
			if m.downloadIndex >= len(m.resources)-1 {
				break
			}
			cookieUrl := m.inputs[1].Value()
			folderDir := m.inputs[5].Value()
			msg, err := m.client.DownloadResource(context.Background(), m.resources[m.downloadIndex], cookieUrl, folderDir)
			sub <- DownloadResourceResp{Err: err, Msg: msg, Index: m.downloadIndex}
			m.downloadIndex++
		}
		return DownloadResourceResp{Err: nil, Msg: "", Index: m.downloadIndex}
	}
}

// A command that waits for the activity on the channel.
func waitForActivity(sub chan DownloadResourceResp) tea.Cmd {
	return func() tea.Msg {
		return DownloadResourceResp(<-sub)
	}
}

func (m *model) fetchResources() tea.Cmd {

	bucketUrl := m.inputs[0].Value()
	cookieUrl := m.inputs[1].Value()
	bucketQuery := buildBucketQuery(m.inputs)
	ignoreText := m.inputs[4].Value()

	return func() tea.Msg {
		resources, err := m.client.SearchBucket(context.Background(), bucketUrl, bucketQuery, cookieUrl, ignoreText)
		if err != nil {
			return GotResources{Err: err, Resources: resources}
		}
		return GotResources{Resources: resources}
	}
}

func (m *model) updateInputs(msg tea.Msg) tea.Cmd {
	cmds := make([]tea.Cmd, len(m.inputs))

	// Only text inputs with Focus() set will respond, so it's safe to simply
	// update all of them here without any further logic.
	for i := range m.inputs {
		m.inputs[i], cmds[i] = m.inputs[i].Update(msg)
	}

	return tea.Batch(cmds...)
}

func (m *model) saveInputs() {
	// Create or overwrite the file
	file, err := os.Create("last-inputs.txt")
	if err != nil {
		panic(err)
	}
	defer file.Close()

	// Write each element of the slice in a new line
	w := bufio.NewWriter(file)
	for _, line := range m.inputs {
		_, err := w.WriteString(line.Value() + "\n")
		if err != nil {
			panic(err)
		}
	}
	w.Flush()
}

func (m *model) preloadLastInputs() {
	// Open the file
	file, err := os.Open("last-inputs.txt")
	if err != nil {
		return
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	lineNumber := 0
	for scanner.Scan() {
		if lineNumber < len(m.inputs) {
			m.inputs[lineNumber].SetValue(scanner.Text())
			lineNumber++
		}
	}
	if err := scanner.Err(); err != nil {
		panic(err)
	}
}

func (m *model) saveLastDownloadKey() {
	file, err := os.OpenFile("last-download-key.csv", os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		panic(err)
	}
	defer file.Close()

	// Create a new writer for the CSV file
	writer := csv.NewWriter(file)
	defer writer.Flush()

	// Write the values talentId and key to the CSV file
	if err := writer.Write([]string{m.inputs[0].Value(), m.resources[m.downloadCount-1]}); err != nil {
		panic(err)
	}
}

func buildBucketQuery(inputs []textinput.Model) string {
	bucketQuery := fmt.Sprintf("?prefix=%s&marker=%s", inputs[2].Value(), inputs[3].Value())
	return bucketQuery
}

type model struct {
	cursor int
	err    error
	sub    chan DownloadResourceResp

	focusIndex int
	inputs     []textinput.Model
	cursorMode textinput.CursorMode

	showTypingView      bool
	showLoadingView     bool
	showDownloadingView bool
	showErrorView       bool
	finishedDownloading bool

	spinner         spinner.Model
	resources       []string
	downloadIndex   int
	downloadCount   int
	lastDownloadKey string
	width           int
	height          int

	progress progress.Model

	client *client.Client
}

func initialModel() model {
	s := spinner.NewModel()
	s.Spinner = spinner.Dot

	p := progress.New(
		progress.WithDefaultGradient(),
		progress.WithWidth(40),
		progress.WithoutPercentage(),
	)

	m := model{

		err: nil,
		sub: make(chan DownloadResourceResp),

		inputs: make([]textinput.Model, 6),

		spinner:             s,
		progress:            p,
		showTypingView:      true,
		showLoadingView:     false,
		showDownloadingView: false,
		showErrorView:       false,
		downloadIndex:       0,
		downloadCount:       0,
		lastDownloadKey:     "",

		client: &client.Client{HTTPClient: http.DefaultClient},
	}

	var t textinput.Model
	for i := range m.inputs {
		t = textinput.New()
		t.CursorStyle = cursorStyle
		t.CharLimit = 32

		switch i {
		case 0:
			t.Placeholder = "Bucket URL"
			t.Focus()
			t.PromptStyle = focusedStyle
			t.TextStyle = focusedStyle
			t.CharLimit = 150
		case 1:
			t.Placeholder = "Site to grab bucket cookie authorizations"
			t.CharLimit = 150
		case 2:
			t.Placeholder = "Bucket resource prefix"
			t.CharLimit = 150
		case 3:
			t.Placeholder = "Start download marker"
			t.CharLimit = 150
		case 4:
			t.Placeholder = "Files to ignore regex"
			t.CharLimit = 150
		case 5:
			t.Placeholder = "Folder to save to"
			t.CharLimit = 150
		}

		m.inputs[i] = t
	}
	return m
}

func main() {
	p := tea.NewProgram(initialModel())
	if _, err := p.Run(); err != nil {
		log.Fatal(err)
		fmt.Printf("There's been an error: %v", err)
		os.Exit(1)
	}
}
