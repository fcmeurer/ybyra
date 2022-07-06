package main

import (
	"bytes"
	"encoding/json"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"
)

type command string
type displayMode uint8

const (
	displayLeases displayMode = 0
	displayReserv             = 1
)

const (
	configGet    command = "config-get"
	statusGet            = "status-get"
	lease4GetAll         = "lease4-get-all"
	lease4Del            = "lease4-del"
)

type KeaRequest[T any] struct {
	Arguments T        `json:"arguments"`
	Command   command  `json:"command"`
	Service   []string `json:"service""`
}

type KeaResponse struct {
	Arguments map[string]json.RawMessage `json:"arguments,omitempty"`
	Result    int                        `json:"result"`
	Text      string                     `json:"text,omitempty"`
}

type KeaStatus struct {
	HighAvailability      map[string]json.RawMessage `json:"high-availability"`
	Result                int                        `json:"result"`
	MultiThreadingEnabled bool                       `json:"multi-threading-enabled"`
	Pid                   int                        `json:"pid"`
	Reload                int                        `json:"reload"`
	Uptime                int                        `json:"uptime"`
}

type Subnet4 struct {
	FourSixInterface   string                     `json:"4o6-interface"`
	FourSixInterfaceId string                     `json:"4o6-interface-id"`
	FourSixSubnet      string                     `json:"4o6-subnet"`
	CalculateTeeTimes  bool                       `json:"calculate-tee-times"`
	Id                 int                        `json:"id"`
	OptionData         []json.RawMessage          `json:"option-data"`
	Pools              []json.RawMessage          `json:"pools"`
	RebindTime         int                        `json:"rebind-timer"`
	Relay              map[string]json.RawMessage `json:"relay"`
	RenewTimer         int                        `json:"renew-timer"`
	Reservations       []Reservation              `json:"reservations"`
	StoreExtendedInfo  bool                       `json:"store-extended-info"`
	Subnet             string                     `json:"subnet"`
	// T1Percent float32 `json:"t1-percent"`
	// T2Percent float32 `json:"t2-percent"`
	// ValidLifetime int `json:"valid-lifetime"`
}

type Lease4 struct {
	ClientId  string `json:"client-id"`
	Cltt      int64  `json:"cltt"`
	FqdnFwd   bool   `json:"fqdn-fwd"`
	FqdnRev   bool   `json:"fqdn-rev"`
	Hostname  string `json:"hostname"`
	HwAddress string `json:"hw-address"`
	IpAddress string `json:"ip-address"`
	State     int    `json:"state"`
	SubnetId  int    `json:"subnet-id"`
	ValidLft  int    `json:"valid-lft"`
}

type Reservation struct {
	BootFileName   string            `json:"boot-file-name"`
	ClientClasses  []json.RawMessage `json:"client-classes"`
	Hostname       string            `json:"hostname"`
	HwAddress      string            `json:"hw-address"`
	IpAddress      string            `json:"ip-address"`
	NextServer     string            `json:"next-server"`
	OptionData     []json.RawMessage `json:"option-data"`
	ServerHostname string            `json:"server-hostname"`
}

func LeaseState(state int) (string, tcell.Color) {
	switch state {
	case 0:
		return "default", tcell.ColorGreen
	case 1:
		return "declined", tcell.ColorRed
	case 2:
		return "expired-reclaimed", tcell.ColorYellow
	}
	return "", tcell.ColorWhite
}

func getSubnets(url string) []Subnet4 {
	jsonbytes := sendCommand(url, configGet, "")
	var grades []KeaResponse
	err := json.Unmarshal(jsonbytes, &grades)
	if err != nil {
		panic(err)
	}
	var dhcp map[string]json.RawMessage
	err = json.Unmarshal(grades[0].Arguments["Dhcp4"], &dhcp)
	if err != nil {
		panic(err)
	}
	var subnets []Subnet4
	err = json.Unmarshal(dhcp["subnet4"], &subnets)
	if err != nil {
		panic(err)
	}
	return subnets
}

func getLeases(url string, subnet int) []Lease4 {
	args := map[string][]int{"subnets": []int{subnet}}
	jsonbytes := sendCommand(url, lease4GetAll, args)
	var grades []KeaResponse
	err := json.Unmarshal(jsonbytes, &grades)
	if err != nil {
		panic(err)
	}
	var leases []Lease4
	err = json.Unmarshal(grades[0].Arguments["leases"], &leases)
	if err != nil {
		panic(err)
	}
	return leases
}

func sendCommand[T any](url string, comm command, args T) []byte {
	keacomm := KeaRequest[T]{
		Command:   comm,
		Arguments: args,
		Service:   []string{"dhcp4"}}
	reqBody, err := json.MarshalIndent(keacomm, "", " ")
	if err != nil {
		panic(err)
	}
	// fmt.Println(string(reqBody))
	resp, err := http.Post(url,
		"application/json", bytes.NewBuffer(reqBody))
	if err != nil {
		panic(err)
	}
	body, err := ioutil.ReadAll(resp.Body)
	defer resp.Body.Close()
	if err != nil {
		panic(err)
	}
	return body
}

func DelLease(url string, ip string) (int, string) {
	args := map[string]string{"ip-address": ip}
	result := sendCommand(url, lease4Del, args)
	var resp []KeaResponse
	err := json.Unmarshal(result, &resp)
	if err != nil {
		panic(err)
	}
	return resp[0].Result, resp[0].Text
}

func UpdateTable(url string, dispmode displayMode, subnet *Subnet4, table *tview.Table) {
	table.Clear()
	switch dispmode {
	case displayLeases:
		table.SetCell(0, 0, tview.NewTableCell("Hostname").SetTextColor(tcell.ColorYellow))
		table.SetCell(0, 1, tview.NewTableCell("IP").SetTextColor(tcell.ColorYellow))
		table.SetCell(0, 2, tview.NewTableCell("MAC").SetTextColor(tcell.ColorYellow))
		table.SetCell(0, 3, tview.NewTableCell("State").SetTextColor(tcell.ColorYellow))
		table.SetCell(0, 4, tview.NewTableCell("Timestamp").SetTextColor(tcell.ColorYellow))
		table.SetCell(0, 5, tview.NewTableCell("Client ID").SetTextColor(tcell.ColorYellow))
		for i, l := range getLeases(url, subnet.Id) {
			t := time.Unix(l.Cltt, 0)
			stateText, stateColor := LeaseState(l.State)
			table.SetCell(i+1, 0, tview.NewTableCell(l.Hostname))
			table.SetCell(i+1, 1, tview.NewTableCell(l.IpAddress))
			table.SetCell(i+1, 2, tview.NewTableCell(l.HwAddress))
			table.SetCell(i+1, 3, tview.NewTableCell(stateText).SetTextColor(stateColor))
			table.SetCell(i+1, 4, tview.NewTableCell(t.Format("2006-01-02T15:04:05")))
			table.SetCell(i+1, 5, tview.NewTableCell(l.ClientId))
		}
	case displayReserv:
		table.SetCell(0, 0, tview.NewTableCell("IP").SetTextColor(tcell.ColorYellow))
		table.SetCell(0, 1, tview.NewTableCell("MAC").SetTextColor(tcell.ColorYellow))
		table.SetCell(0, 2, tview.NewTableCell("Hostname").SetTextColor(tcell.ColorYellow))
		table.SetCell(0, 3, tview.NewTableCell("Bootfile").SetTextColor(tcell.ColorYellow))
		table.SetCell(0, 4, tview.NewTableCell("Next Server").SetTextColor(tcell.ColorYellow))
		table.SetCell(0, 5, tview.NewTableCell("Server Hostname").SetTextColor(tcell.ColorYellow))
		for i, l := range subnet.Reservations {
			table.SetCell(i+1, 0, tview.NewTableCell(l.IpAddress))
			table.SetCell(i+1, 1, tview.NewTableCell(l.HwAddress))
			table.SetCell(i+1, 2, tview.NewTableCell(l.Hostname))
			table.SetCell(i+1, 3, tview.NewTableCell(l.BootFileName))
			table.SetCell(i+1, 4, tview.NewTableCell(l.NextServer))
			table.SetCell(i+1, 5, tview.NewTableCell(l.ServerHostname))
		}
	}
	table.ScrollToBeginning()
}

func SearchForwardList(input *tview.InputField, list *tview.List, line *tview.TextView) {
	for _, i := range list.FindItems(input.GetText(), "", false, false) {
		if i > list.GetCurrentItem() {
			line.SetText("/" + input.GetText())
			list.SetCurrentItem(i)
			return
		}
	}
	line.SetText("Pattern not found \"" + input.GetText() + "\"")
}

func SearchForwardTable(input *tview.InputField, table *tview.Table, line *tview.TextView) {
	curr, _ := table.GetSelection()
	for i := curr + 1; i < table.GetRowCount(); i++ {
		for j := 0; j < table.GetColumnCount(); j++ {
			if strings.Contains(table.GetCell(i, j).Text, input.GetText()) {
				table.SetSelectable(true, false)
				table.Select(i, 0)
				line.SetText("/" + input.GetText())
				return
			}
		}
	}
	line.SetText("Pattern not found \"" + input.GetText() + "\"")
}

func main() {
	url := "http://127.0.0.1:8000"
	if len(os.Args) > 1 {
		url = "http://" + os.Args[1] + ":8000"
	}
	dispmode := displayLeases
	subnets := getSubnets(url)
	// Sorts the subnets by IP
	sort.Slice(subnets, func(i, j int) bool {
		return bytes.Compare(
			net.ParseIP(strings.Split(subnets[i].Subnet, "/")[0]),
			net.ParseIP(strings.Split(subnets[j].Subnet, "/")[0])) < 0
	})
	table := tview.NewTable().
		SetSeparator(tview.Borders.Vertical).
		SetBorders(false).
		SetSelectable(false, false)
	table.SetBorder(true)
	table.SetTitle("Leases")
	app := tview.NewApplication()
	statusline := tview.NewTextView().SetText(url)
	statusinput := tview.NewInputField()
	statuspage := tview.NewPages().
		AddPage("line", statusline, true, true).
		AddPage("input", statusinput, true, false)
	subnetList := tview.NewList().
		ShowSecondaryText(false)
	subnetList.SetBorder(true)
	subnetList.SetTitle("Subnets")
	var prev tview.Primitive
	prev = subnetList
	for _, x := range subnets {
		subnetList.AddItem(x.Subnet, "", 0, nil)
	}
	subnetList.SetSelectedFunc(func(index int, text string, stext string, r rune) {
		UpdateTable(url, dispmode, &subnets[index], table)
	})
	statusinput.SetFinishedFunc(func(key tcell.Key) {
		statuspage.SwitchToPage("line")
		app.SetFocus(prev)
		switch prev {
		case subnetList:
			SearchForwardList(statusinput, subnetList, statusline)
		case table:
			SearchForwardTable(statusinput, table, statusline)
		}
	})

	grid := tview.NewGrid().
		SetColumns(0, -5).
		SetRows(0, 1).
		SetBorders(false).
		AddItem(subnetList, 0, 0, 1, 1, 0, 0, true).
		AddItem(table, 0, 1, 1, 1, 0, 0, false).
		AddItem(statuspage, 1, 0, 1, 2, 0, 0, false)

	subnetList.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Key() == tcell.KeyTab {
			app.SetFocus(table)
			return nil
		}
		if event.Rune() == 'l' {
			app.SetFocus(table)
			return nil
		}
		if event.Key() == tcell.KeyRight {
			app.SetFocus(table)
			return nil
		}
		if event.Rune() == 'j' {
			return tcell.NewEventKey(tcell.KeyDown, 258, tcell.ModNone)
		}
		if event.Rune() == 'k' {
			return tcell.NewEventKey(tcell.KeyUp, 257, tcell.ModNone)
		}
		if event.Rune() == 'n' {
			SearchForwardList(statusinput, subnetList, statusline)
			return event
		}
		if event.Rune() == 'N' {
			indexes := subnetList.FindItems(statusinput.GetText(), "", false, false)
			curr := subnetList.GetCurrentItem()
			for j, i := range indexes {
				if i >= curr && j > 0 {
					statusline.SetText("?" + statusinput.GetText())
					subnetList.SetCurrentItem(indexes[j-1])
					if indexes[j-1] == curr {
						statusline.SetText("Pattern not found \"" + statusinput.GetText() + "\"")
					}
					return event
				}
			}
			statusline.SetText("Pattern not found \"" + statusinput.GetText() + "\"")
			return event
		}
		if event.Rune() == '/' {
			statuspage.SwitchToPage("input")
			prev = subnetList
			app.SetFocus(statuspage)
			return nil
		}
		return event
	})

	table.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Key() == tcell.KeyTab {
			app.SetFocus(subnetList)
			return nil
		}
		_, col := table.GetOffset()
		if col < 1 {
			if event.Rune() == 'h' {
				app.SetFocus(subnetList)
				return nil
			}
			if event.Key() == tcell.KeyLeft {
				app.SetFocus(subnetList)
				return nil
			}
		}
		if event.Rune() == 'n' {
			SearchForwardTable(statusinput, table, statusline)
			return event
		}
		if event.Rune() == 'N' {
			curr, _ := table.GetSelection()
			for i := curr - 1; i > 0; i-- {
				for j := 0; j < table.GetColumnCount(); j++ {
					if strings.Contains(table.GetCell(i, j).Text, statusinput.GetText()) {
						table.SetSelectable(true, false)
						table.Select(i, 0)
						statusline.SetText("?" + statusinput.GetText())
						return event
					}
				}
			}
			statusline.SetText("Pattern not found \"" + statusinput.GetText() + "\"")
			return event
		}
		if selectable, _ := table.GetSelectable(); event.Rune() == 'd' && selectable && dispmode == displayLeases {
			row, _ := table.GetSelection()
			ipaddr := table.GetCell(row, 1).Text
			_, text := DelLease(url, ipaddr)
			statusline.SetText(text)
			return nil
		}
		if event.Key() == tcell.KeyEnter {
			row, _ := table.GetSelectable()
			table.SetSelectable(!row, false)
		}
		if event.Rune() == '/' {
			statuspage.SwitchToPage("input")
			prev = table
			app.SetFocus(statuspage)
			return nil
		}
		return event
	})

	statusinput.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Key() == tcell.KeyEscape {
			statuspage.SwitchToPage("line")
			app.SetFocus(prev)
			return nil
		}
		return event
	})

	grid.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if (event.Rune() == 'q' || event.Key() == tcell.KeyEscape) && !statuspage.HasFocus() {
			app.Stop()
			return nil
		}
		if event.Rune() == 'm' {
			dispmode = (dispmode + 1) % 2
			UpdateTable(url,
				dispmode,
				&subnets[subnetList.GetCurrentItem()],
				table)
			switch dispmode {
			case displayLeases:
				table.SetTitle("Leases")
			case displayReserv:
				table.SetTitle("Reservations")
			}
		}
		return event
	})

	if err := app.SetRoot(grid, true).SetFocus(grid).Run(); err != nil {
		panic(err)
	}
}
