package tui

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"time"

	linkedlist "container/list"

	"github.com/HomelessHunter/p2p-chat/client/node"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/libp2p/go-libp2p-core/protocol"
)

const (
	MENU      string = "Menu"
	CHAT      string = "Chat"
	SETTINGS  string = "Settings"
	QUIT      string = "Quit"
	CONNECT   string = "Connect to chat room"
	CREATE    string = "Create chat room"
	CONNECTED string = "Connected"
	CREATED   string = "Created"
)

var (
	docStyle          = lipgloss.NewStyle().Margin(1, 2)
	titleStyle        = lipgloss.NewStyle().MarginLeft(2)
	itemStyle         = lipgloss.NewStyle().PaddingLeft(4)
	selectedItemStyle = lipgloss.NewStyle().PaddingLeft(2).Foreground(lipgloss.Color("170"))
	paginationStyle   = list.DefaultStyles().PaginationStyle.PaddingLeft(4)
	helpStyle         = list.DefaultStyles().HelpStyle.PaddingLeft(4).PaddingBottom(1)
	quitTextStyle     = lipgloss.NewStyle().Margin(1, 0, 2, 4)
)

type item string

func (i item) FilterValue() string {
	return ""
}

type itemDelegate struct{}

func (d itemDelegate) Height() int                               { return 1 }
func (d itemDelegate) Spacing() int                              { return 1 }
func (d itemDelegate) Update(msg tea.Msg, m *list.Model) tea.Cmd { return nil }
func (d itemDelegate) Render(w io.Writer, m list.Model, index int, listItem list.Item) {
	i, ok := listItem.(item)
	if !ok {
		return
	}

	str := fmt.Sprintf("%d. %s", index+1, i)

	fn := itemStyle.Render
	if index == m.Index() {
		fn = func(s string) string {
			return selectedItemStyle.Render(">" + s)
		}
	}

	fmt.Fprintf(w, fn(str))
}

type msgReceiver struct{}

type errMsg struct {
	err error
}

func (err errMsg) Error() string {
	return err.Error()
}

type model struct {
	mainList   list.Model
	chatList   list.Model
	choices    *linkedlist.List
	textInput  textinput.Model
	addrInput  textinput.Model
	node       *node.Node
	ctx        *ctx
	msgReadCh  chan []byte
	msgWriteCh chan []byte
	closeCtx   *ctx
	buf        *bytes.Buffer
	err        error
}

type ctx struct {
	context.Context
	context.CancelFunc
}

func InitialModel(contx context.Context, cancel context.CancelFunc, port int, pnKey string) *model {
	msgReadCh := make(chan []byte, 10)
	msgWriteCh := make(chan []byte, 10)
	node, err := node.NewNode(contx, port, pnKey)
	if err != nil {
		panic(err)
	}
	contxt := &ctx{contx, cancel}
	mainItems := []list.Item{
		item(CHAT),
		item(SETTINGS),
		item(QUIT),
	}
	chatItems := []list.Item{
		item(CREATE),
		item(CONNECT),
	}
	ti := constructTextView("Message", 500, 50)
	addri := constructTextView("Insert the address", 128, 50)
	model := model{
		mainList:   list.New(mainItems, itemDelegate{}, 10, 20),
		chatList:   list.New(chatItems, itemDelegate{}, 10, 20),
		textInput:  ti,
		addrInput:  addri,
		choices:    linkedlist.New(),
		node:       node,
		ctx:        contxt,
		msgReadCh:  msgReadCh,
		msgWriteCh: msgWriteCh,
		buf:        new(bytes.Buffer),
	}
	model.mainList = styleList(model.mainList, "P2P_CHAT")
	model.chatList = styleList(model.chatList, CHAT)
	model.choices.PushBack(item(MENU))

	return &model
}

func (m *model) Init() tea.Cmd {
	return nil
}

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	i, ok := m.choices.Back().Value.(item)
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyCtrlC:
			return m, tea.Quit
		case tea.KeyEnter:
			switch string(i) {
			// PushBack item from main menu list
			case MENU:
				i, ok = m.mainList.SelectedItem().(item)
				if ok {
					m.choices.PushBack(i)
				}
				if string(i) == QUIT {
					cmd = tea.Quit
				}
				return m, cmd
			// PushBack item from chat menu list
			case CHAT:
				i, ok = m.chatList.SelectedItem().(item)
				if ok {
					m.choices.PushBack(i)
				}
				switch string(i) {
				// case CONNECT:
				// 	cmd = connectChat(m)
				case CREATE:
					cmd = createChat(m)
				}
				return m, cmd
			case CONNECT:
				m.choices.PushBack(item(CONNECTED))
				cmd = connectChat(m)
			case CONNECTED, CREATED:
				sendMsg(m, []byte(m.textInput.Value()))
			}
			return m, cmd
		case tea.KeyEsc:
			if ok {
				switch string(i) {
				case CONNECTED:
					m.closeCtx.CancelFunc()
				case CREATED:
					m.closeCtx.CancelFunc()
					removeChat(m)
				}
				m.choices.Remove(m.choices.Back())
			}
			return m, cmd
		}
	case tea.WindowSizeMsg:
		h, v := docStyle.GetFrameSize()
		switch string(i) {
		case CHAT:
			m.chatList.SetSize(msg.Width-h, msg.Height-v)
		default:
			m.mainList.SetSize(msg.Width-h, msg.Height-v)
		}
		return m, cmd
	case msgReceiver:
		switch string(i) {
		case CONNECTED, CREATED:
			select {
			case <-m.closeCtx.Done():
			default:
				cmd = rcvMsg(m)
			}
		default:
		}
		return m, cmd
	case errMsg:
		if ok {
			m.choices.Remove(m.choices.Back())
		}
		// panic("error")
		// m.node.Close()
		return m, cmd
	}
	switch string(i) {
	case CHAT:
		m.chatList, cmd = m.chatList.Update(msg)
	case CONNECT:
		m.addrInput, cmd = m.addrInput.Update(msg)
	case CONNECTED, CREATED:
		m.textInput, cmd = m.textInput.Update(msg)
	default:
		m.mainList, cmd = m.mainList.Update(msg)
	}
	return m, cmd
}

func (m *model) View() string {
	if choice := m.choices.Back(); choice != nil {
		switch item := choice.Value.(item); string(item) {
		case MENU:
			return m.mainList.View()
		case CHAT:
			return m.chatList.View()
		case SETTINGS:
		case CONNECT:
			// m.choices.PushBack(item)
			return m.addrInput.View()
		case CREATE:
			// insert buffer before
			return ":)"
		case CONNECTED:
			return fmt.Sprintf("\n%s\n\n%s", m.buf.String(), m.textInput.View())
		case CREATED:
			return fmt.Sprintf("\n%s\n\n%s", m.buf.String(), m.textInput.View())
		}
	}
	return string(m.choices.Back().Value.(item))
}

func constructTextView(placeholder string, charLimit int, width int) textinput.Model {
	ti := textinput.New()
	ti.Placeholder = placeholder
	ti.Focus()
	ti.CharLimit = charLimit
	ti.Width = width

	return ti
}

func styleList(list list.Model, title string) list.Model {
	list.Title = title
	list.SetShowStatusBar(false)
	list.SetFilteringEnabled(false)
	list.Styles.Title = titleStyle
	list.Styles.PaginationStyle = paginationStyle
	list.Styles.HelpStyle = helpStyle

	return list
}

func createChat(m *model) tea.Cmd {
	contxt, cancel := context.WithCancel(m.ctx.Context)
	m.closeCtx = &ctx{contxt, cancel}
	m.node.SetDefaultHandlers(m.closeCtx.Context, m.msgReadCh, m.msgWriteCh)
	addrs, err := m.node.NodeAddrs()
	if err != nil {
		return func() tea.Msg {
			return errMsg{err}
		}
	}
	sendMsg(m, []byte(fmt.Sprintf("%s/p2p/%s", addrs[0].Addrs[0].String(), addrs[0].ID.String())))

	m.choices.PushBack(item(CREATED))

	return rcvMsg(m)
}

func removeChat(m *model) {
	m.node.DeleteHandler(node.CHATPID)
}

func connectChat(m *model) tea.Cmd {
	return func() tea.Msg {
		contxt, cancel := context.WithCancel(m.ctx.Context)
		m.closeCtx = &ctx{contxt, cancel}
		peerID, err := m.node.EstablishNewConn(m.ctx.Context, m.addrInput.Value())
		if err != nil {
			return errMsg{err}
		}
		err = m.node.OpenNewStream(m.ctx, m.msgReadCh, m.msgWriteCh, peerID, protocol.ID(node.CHATPID))
		if err != nil {
			return errMsg{err}
		}

		return rcvMsg(m)()
	}
}

func rcvMsg(m *model) tea.Cmd {
	return func() tea.Msg {
		select {
		case msg := <-m.msgReadCh:
			m.buf.Write(msg)
		case <-time.After(100 * time.Millisecond):
		}
		return msgReceiver{}
	}
}

// func receiveMsg(m model) {
// 	for {
// 		select {
// 		case <-m.ctx.Done():
// 			return
// 		case <-m.closeReadCh:
// 			return
// 		case msg := <-m.msgReadCh:
// 			m.buf.Write(msg)
// 		}
// 	}
// }

func sendMsg(m *model, msg []byte) {
	newMsg := make([]byte, len(msg)+1)
	if len(msg) > 0 && string(msg) != "" {
		copy(newMsg, msg)
		newMsg[len(newMsg)-1] = '\n'
		m.msgWriteCh <- newMsg
		m.buf.Write(msg)
	}
}
