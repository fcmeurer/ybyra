package main

import (
	"bytes"
	"encoding/json"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"io/ioutil"
	"net/http"
	"os"
	"time"
)

type command string

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
	Reservations       []json.RawMessage          `json:"reservations"`
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

func main() {
	url := "http://127.0.0.1:8000"
	if len(os.Args) > 1 {
		url = "http://" + os.Args[1] + ":8000"
	}
	// body := sendCommand(url, statusGet, "")
	// fmt.Println(string(body))
	subnets := getSubnets(url)
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

	for _, x := range subnets {
		subnetList.AddItem(x.Subnet, "", 0, nil)
	}
	subnetList.SetSelectedFunc(func(index int, text string, stext string, r rune) {
		leases := getLeases(url, index+1)
		table.Clear()
		table.SetCell(0, 0, tview.NewTableCell("Hostname").SetTextColor(tcell.ColorYellow))
		table.SetCell(0, 1, tview.NewTableCell("IP").SetTextColor(tcell.ColorYellow))
		table.SetCell(0, 2, tview.NewTableCell("MAC").SetTextColor(tcell.ColorYellow))
		table.SetCell(0, 3, tview.NewTableCell("State").SetTextColor(tcell.ColorYellow))
		table.SetCell(0, 4, tview.NewTableCell("Timestamp").SetTextColor(tcell.ColorYellow))
		table.SetCell(0, 5, tview.NewTableCell("Client ID").SetTextColor(tcell.ColorYellow))

		for i, l := range leases {
			t := time.Unix(l.Cltt, 0)
			stateText, stateColor := LeaseState(l.State)
			table.SetCell(i+1, 0, tview.NewTableCell(l.Hostname))
			table.SetCell(i+1, 1, tview.NewTableCell(l.IpAddress))
			table.SetCell(i+1, 2, tview.NewTableCell(l.HwAddress))
			table.SetCell(i+1, 3, tview.NewTableCell(stateText).SetTextColor(stateColor))
			table.SetCell(i+1, 4, tview.NewTableCell(t.Format("2006-01-02T15:04:05")))
			table.SetCell(i+1, 5, tview.NewTableCell(l.ClientId))
		}
		table.ScrollToBeginning()
	})

	// statusinput.SetFinishedFunc(func (key tcell.Key) {

	// })

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
		if event.Rune() == '/' {
			statuspage.SwitchToPage("input")
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
		if selectable, _ := table.GetSelectable(); event.Rune() == 'd' && selectable{
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

		return event
	})

	statusinput.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Key() == tcell.KeyEscape {
			statuspage.SwitchToPage("line")
			app.SetFocus(subnetList)
			return nil
		}
		return event
	})

	grid.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if (event.Rune() == 'q' || event.Key() == tcell.KeyEscape) && !statuspage.HasFocus() {
			app.Stop()
			return nil
		}
		return event
	})

	if err := app.SetRoot(grid, true).SetFocus(grid).Run(); err != nil {
		panic(err)
	}
}
