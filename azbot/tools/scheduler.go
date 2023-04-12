package tools

import (
	"github.com/ezquant/azbot/azbot"
	"github.com/ezquant/azbot/azbot/service"
	"github.com/samber/lo"
	log "github.com/sirupsen/logrus"
)

type OrderCondition struct {
	Condition func(df *azbot.Dataframe) bool
	Size      float64
	Side      azbot.SideType
}

type Scheduler struct {
	pair            string
	orderConditions []OrderCondition
}

func NewScheduler(pair string) *Scheduler {
	return &Scheduler{pair: pair}
}

func (s *Scheduler) SellWhen(size float64, condition func(df *azbot.Dataframe) bool) {
	s.orderConditions = append(
		s.orderConditions,
		OrderCondition{Condition: condition, Size: size, Side: azbot.SideTypeSell},
	)
}

func (s *Scheduler) BuyWhen(size float64, condition func(df *azbot.Dataframe) bool) {
	s.orderConditions = append(
		s.orderConditions,
		OrderCondition{Condition: condition, Size: size, Side: azbot.SideTypeBuy},
	)
}

func (s *Scheduler) Update(df *azbot.Dataframe, broker service.Broker) {
	s.orderConditions = lo.Filter[OrderCondition](s.orderConditions, func(oc OrderCondition, _ int) bool {
		if oc.Condition(df) {
			_, err := broker.CreateOrderMarket(oc.Side, s.pair, oc.Size)
			if err != nil {
				log.Error(err)
				return true
			}
			return false
		}
		return true
	})
}
