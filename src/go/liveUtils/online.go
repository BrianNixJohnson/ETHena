package liveUtils

import (
	. "TradingHackathon/src/go/rsi"
	"context"
	"fmt"
	"time"

	luno "github.com/luno/luno-go"
	"github.com/luno/luno-go/decimal"
)

// function to cancel most recent order
func cancelPrevOrder(b *RsiBot) {
	if b.PrevOrder == "" {
		return
	}
	time.Sleep(time.Second * 2)
	checkReq := luno.GetOrderRequest{Id: b.PrevOrder}
	checkRes, err := Client.GetOrder(context.Background(), &checkReq)
	if err != nil {
		panic(err)
	}
	if checkRes.State == "PENDING" {
		time.Sleep(time.Second * 2)
		req := luno.StopOrderRequest{OrderId: b.PrevOrder}
		res, err := Client.StopOrder(context.Background(), &req)
		if err != nil {
			panic(err)
		}
		if res.Success {
			fmt.Println("Successfully cancelled previous order")
		} else {
			fmt.Println("ERROR! Failed to cancel previous order")
			cancelPrevOrder(b)
		}
	}
	fmt.Println("Previous order was filled. Cancellation not required.")
}

// function to execute buying of items
func buy(b *RsiBot, currAsk decimal.Decimal) {
	cancelPrevOrder(b)
	time.Sleep(time.Second * 2)
	startStock, startFunds := getAssets(PairName[:3], PairName[3:])
	price := currAsk.Sub(decimal.NewFromFloat64(0.00000001, 8))
	buyableStock := startFunds.Div(price, 8)

	switch PairName[:3] {
	case "BCH":
		buyableStock = buyableStock.Mul(decimal.NewFromFloat64(0.1003, 8))
	case "ETH":
		buyableStock = buyableStock.Mul(decimal.NewFromFloat64(0.8013, 8))
	case "LTC":
		buyableStock = buyableStock.Mul(decimal.NewFromFloat64(0.0602, 8))
	case "XRP":
		buyableStock = buyableStock.Mul(decimal.NewFromFloat64(0.0381, 8))
	}

	buyableStock = buyableStock.ToScale(0)
	// checking if there are no funds available
	if buyableStock.Sign() == 0 {
		fmt.Println("Not enough funds available")
		return
	}
	//Create limit order
	req := luno.PostLimitOrderRequest{
		Pair:   PairName,
		Price:  price,
		Type:   "BID", //We are putting in a bid to buy at the ask price
		Volume: buyableStock,
		//BaseAccountId: --> Not needed until using multiple strategies
		//CounterAccountId: --> Same as above
		PostOnly: true,
	}
	res, err := Client.PostLimitOrder(context.Background(), &req)
	for err != nil {
		fmt.Println(err)
		time.Sleep(time.Second * 30)
		res, err = Client.PostLimitOrder(context.Background(), &req)
	}
	fmt.Println("BUY - order ", res.OrderId, " placed at ", price)
	b.PrevOrder = res.OrderId
	b.ReadyToBuy = false
	b.TradesMade++
	b.StopLoss = price
	b.BuyPrice = price
	// wait till order has gone through
	fmt.Println("Waiting for buy order to be partially filled")
	for {
		time.Sleep(2 * time.Second)
		if startStock.Cmp(getAsset(PairName[:3])) == -1 {
			fmt.Println("Buy order has been partially filled")
			return
		}
	}
}

func sell(b *RsiBot, currBid decimal.Decimal) {
	cancelPrevOrder(b)
	time.Sleep(time.Second * 2)
	startStock, startFunds := getAssets(PairName[:3], PairName[3:])
	price := currBid.Add(decimal.NewFromFloat64(0.00000001, 8))
	req := luno.PostLimitOrderRequest{
		Pair:   PairName,
		Price:  price,
		Type:   "ASK", //We are putting in a ask to sell at the bid price
		Volume: startStock,
		//BaseAccountId: --> Not needed until using multiple strategies
		//CounterAccoundId: --> Same as above
		PostOnly: true,
	}
	res, err := Client.PostLimitOrder(context.Background(), &req)
	for err != nil {
		fmt.Println(err)
		time.Sleep(2 * time.Second)
		res, err = Client.PostLimitOrder(context.Background(), &req)
	}

	fmt.Println("SELL - order ", res.OrderId, " placed at ", price)
	b.PrevOrder = res.OrderId
	b.ReadyToBuy = true
	b.TradesMade++
	fmt.Println("Waiting for sell order to be partially filled")
	for {
		time.Sleep(2 * time.Second)
		if startFunds.Cmp(getAsset(PairName[3:])) == -1 {
			fmt.Println("Sell order has been partially filled")
			return
		}
	}
}

// function to execute trades using the RSI bot
func TradeLive(b *RsiBot) {
	time.Sleep(20 * time.Second)
	res := getTickerRes()
	currAsk, currBid := res.Ask, res.Bid

	// calculating RSI using RSI algorithm
	var rsi decimal.Decimal
	rsi, b.UpEma, b.DownEma = GetRsi(b.PrevAsk, currAsk, b.UpEma, b.DownEma, b.TradingPeriod)
	// fmt.Println("RSI", rsi, "U:", b.UpEma, "D:", b.DownEma)
	b.PrevAsk = currAsk

	PopulateFile(b, currAsk, currBid, rsi)

	if b.ReadyToBuy { // check if sell order has gone trough
		// fmt.Println("Current Ask", currAsk)
		if rsi.Cmp(b.OverSold) == -1 && rsi.Sign() != 0 {
			buy(b, currAsk)
		}
	} else {
		bound := currBid.Mul(b.StopLossMult)

		// fmt.Println("Current Bid", currBid)
		// fmt.Println("Stop Loss", b.StopLoss)

		if (currBid.Cmp(b.BuyPrice) == 1 && currBid.Cmp(b.StopLoss) == -1) ||
			currBid.Cmp(b.BuyPrice.Mul(decimal.NewFromFloat64(0.98, 8))) == -1 {
			sell(b, currBid)
		} else if bound.Cmp(b.StopLoss) == 1 {
			b.StopLoss = bound
			// fmt.Println("Stoploss changed to: ", b.StopLoss)
		}

	}
	b.NumOfDecisions++

}
