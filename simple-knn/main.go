package main

import (
	"log"

	"os"

	"github.com/apourchet/investment"
	"github.com/apourchet/investment/lib/candelizer"
	"github.com/apourchet/investment/lib/influx-session"
	"github.com/apourchet/investment/lib/sliding-window"
)

// This is the implementation of the very simple
// directionality prediction through knn
// http://www.forexfactory.com/showthread.php?t=516785
// What to predict (what target) -> The direction of the next day (bullish or bearish)
// What to predict it with (which inputs) -> The direction of the previous 2 days
// How to relate the target and inputs (what model) -> KNN on the previous 70 instances

type Trader struct {
	account *invt.Account
	in      chan candelizer.CandleInterface
}

type Instance struct {
	first  bool // Increase on first day?
	second bool // Increase on second day?
	label  bool // Increase on target?
}

type Moment struct {
	Value float64
}

var (
	db             *ix_session.Session
	cand           *candelizer.Candelizer
	instanceWindow slidwin.SlidingWindow
	trainWindow    slidwin.SlidingWindow
)

func NewTrader() *Trader {
	return &Trader{invt.NewAccount(10000), make(chan candelizer.CandleInterface)}
}

func (t *Trader) Start() error {
	log.Println("Trader starting")

	days := 0
	pred := 0
	goods := 0
	bads := 0
	for c := range t.in {
		if c == nil {
			break
		}
		if pred != 0 {
			log.Println("Result:", c.Close()-c.Open(), -pred)
			if (pred > 0 && c.Close() > c.Open()) || (pred < 0 && c.Close() < c.Open()) {
				bads += 1
			} else {
				goods += 1
			}
		}
		instanceWindow.Push(c.Open())
		instanceWindow.Push(c.Close())

		if days > 3 {
			// Create instance and add it to the training window
			ins := Instance{}
			ins.label = instanceWindow[1].(float64) > instanceWindow[0].(float64)
			ins.second = instanceWindow[3].(float64) > instanceWindow[2].(float64)
			ins.first = instanceWindow[5].(float64) > instanceWindow[4].(float64)
			trainWindow.Push(ins)
		}

		if days > 70+3 {
			// Use the full window to predict the next day
			pred = 0
			first := instanceWindow[3].(float64) > instanceWindow[2].(float64)
			second := instanceWindow[1].(float64) > instanceWindow[0].(float64)
			for _, ix := range trainWindow {
				ins := ix.(Instance)
				if ins.first == first && ins.second == second {
					pred += intOfBool(ins.label)
				}
			}
			if pred != 0 {
				db.Write("moment", struct{ Prediction int }{pred}, c.CloseTime())
			}
		}
		db.Write("moment", Moment{c.Close()}, c.CloseTime())
		db.Write("moment", Moment{c.Open()}, c.OpenTime())
		days += 1
	}
	log.Println(goods, bads)
	return nil
}

func intOfBool(b bool) int {
	if b {
		return 1
	}
	return -1
}

func (t *Trader) OnEnd() {
	db.Flush()
	log.Printf("%+v\n", t.account.Stats)
	log.Printf("%+v\n", t.account)
	t.in <- nil
}

func (t *Trader) OnData(record []string, format invt.DataFormat) {
	c := invt.ParseCandleFromRecord("EURUSD", record)
	if c == nil {
		log.Println("Error parsing candle.", record)
		return
	}

	cand.Step(c.Close, c.Timestamp)

	if cand.Steps%(24*60) == 0 {
		t.in <- cand
	}
}

func init() {
	cand = candelizer.NewCandelizer(24 * 60)
	db = ix_session.NewSession(ix_session.DEFAULT_ADDRESS, "investment", "password", "testdb")
	instanceWindow = slidwin.NewSlidingWindow(6) // Each instance consists of 3 days so 6 points (open + close)
	trainWindow = slidwin.NewSlidingWindow(70)   // Each training/classification window is 70 days
}

func main() {
	var datafile string
	if len(os.Args) < 2 {
		log.Fatal("No datafile passed in as argument")
	}
	datafile = os.Args[1]

	trader := NewTrader()
	simulator := invt.NewSimulator(invt.DATAFORMAT_CANDLE, datafile, 0)
	simulator.SimulateDataStream(trader)
}
