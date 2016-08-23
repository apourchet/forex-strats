package main

import (
	"log"

	"os"

	"github.com/apourchet/investment"
	"github.com/apourchet/investment/lib/ema"
	"github.com/apourchet/investment/lib/influx-session"
)

type Trader struct {
	account *invt.Account
	in      chan *invt.Quote
}

type MyPoint struct {
	Bid   float64
	Ema2  float64
	Ema5  float64
	Ema10  float64
}

var (
	db *ix_session.Session
)

func NewTrader() *Trader {
	return &Trader{invt.NewAccount(10000), make(chan *invt.Quote)}
}

func (t *Trader) Start() error {
	log.Println("Trader starting")

	ema2 := ema.NewEma(ema.AlphaFromN(2))
	ema5 := ema.NewEma(ema.AlphaFromN(5))
	ema10 := ema.NewEma(ema.AlphaFromN(10))
	units := int32(3000)
	for q := range t.in {
		if q == nil {
			break
		}
		pt := MyPoint{}
		sold := false
		bought := false
		// units := int32((10000. - t.account.RealizedPl) / 0.2 / 3.)
		if ema2.Value < ema5.Value && ema2.ComputeNext(q.Bid) > ema5.ComputeNext(q.Bid) && // 2 crosses 5 upwards
			ema2.Value < ema10.Value && ema5.Value < ema10.Value && // 2 and 5 cross 10 upwards
			ema2.ComputeNext(q.Bid) > ema10.ComputeNext(q.Bid) &&
			ema5.ComputeNext(q.Bid) > ema10.ComputeNext(q.Bid) {
			invt.TradeQuote(t.account, q, units, invt.SIDE_BUY)
			bought = true
		} else if ema2.Value > ema5.Value && ema2.ComputeNext(q.Bid) < ema5.ComputeNext(q.Bid) && // 2 crosses 5 upwards
			ema2.Value > ema10.Value && ema5.Value > ema10.Value && // 2 and 5 cross 10 upwards
			ema2.ComputeNext(q.Bid) < ema10.ComputeNext(q.Bid) &&
			ema5.ComputeNext(q.Bid) < ema10.ComputeNext(q.Bid) {
			invt.TradeQuote(t.account, q, units, invt.SIDE_SELL)
			sold = true
		}

		if sold || bought {
			db.Write("moment", struct{ RealizedPl float64 }{t.account.RealizedPl}, q.Timestamp)
			db.Write("moment", struct{ Exposure float64 }{t.account.Exposure(nil)}, q.Timestamp)
			if sold {
				db.Write("moment", struct{ OrderSell float64 }{q.Bid}, q.Timestamp)
			} else if bought {
				db.Write("moment", struct{ OrderBuy float64 }{q.Ask}, q.Timestamp)
			}
		}

		pt.Bid = q.Bid
		pt.Ema2 = ema2.Step(q.Bid)
		pt.Ema5 = ema5.Step(q.Bid)
		pt.Ema10 = ema10.Step(q.Bid)
		db.Write("moment", pt, q.Timestamp)
	}
	return nil
}

func (t *Trader) OnEnd() {
	db.Flush()
	log.Printf("%+v\n", t.account.Stats)
	log.Printf("%+v\n", t.account)
	t.in <- nil
}

func (t *Trader) OnData(record []string, format invt.DataFormat) {
	if format == invt.DATAFORMAT_QUOTE {
		q := invt.ParseQuoteFromRecord("EURUSD", record)
		t.in <- q
	} else if format == invt.DATAFORMAT_CANDLE {
		c := invt.ParseCandleFromRecord("EURUSD", record)
		if c == nil {
			log.Println("Error parsing candle.", record)
			return
		}
		q := &invt.Quote{}
		q.Bid = c.Close
		q.Ask = c.Close + 0.00025
		q.InstrumentId = c.InstrumentId
		q.Timestamp = c.Timestamp
		t.in <- q
	}
}

func main() {
	db = ix_session.NewSession(ix_session.DEFAULT_ADDRESS, "investment", "password", "testdb")
	var datafile string
	if len(os.Args) >= 2 {
		datafile = os.Args[1]
	} else {
		log.Fatal("No datafile passed in as argument")
	}
	trader := NewTrader()
	simulator := invt.NewSimulator(invt.DATAFORMAT_CANDLE, datafile, 0)
	simulator.SimulateDataStream(trader)
}
