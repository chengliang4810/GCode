package desktop

import (
	"gcode/internal/contracts"
	"gcode/internal/design"
	"github.com/egoist/mygo/ui"
)

func (a *App) billingSettings(c *ui.Context) {
	a.integrationSubtitle(c, "Credits run your Cloud sessions.")
	b := a.state.Integrations.Billing
	if b.LoadState == contracts.LoadLoading {
		a.integrationLoading(c, "Loading billing")
		return
	}
	if b.LoadState == contracts.LoadError {
		a.settingsCard(c, func() { a.integrationError(c, tr(c, "Couldn't load billing: ")+b.Error, "billing.refresh", nil) })
		return
	}
	if !b.Available {
		a.settingsCard(c, func() {
			a.settingsRow(c, "Billing isn't set up on this server yet, so plans and credits can't be bought here.", "", func() {})
		})
	}
	if b.Notice != "" {
		ui.Text(c, b.Notice).FontSize(design.TextSize(c, design.TextBase)).TextColor(a.palette.Success).Padding(8, 16)
	}
	if b.Checkout != nil && b.Checkout.State == "waiting" {
		a.settingsCard(c, func() {
			a.settingsRow(c, "Finish in your browser — this page updates when it's done.", "", func() {
				textButton(c, tr(c, "Dismiss")).OnClick(func() { a.dispatch(contracts.Command{Kind: "billing.checkout.dismiss"}) })
			})
		})
	}
	sectionTitle(c, tr(c, "Credits"))
	a.settingsCard(c, func() {
		credits := b.Credits
		ui.Column(c).FillWidth().Padding(16).Gap(14).Children(func() {
			ui.Row(c).FillWidth().AlignItems(ui.End).Children(func() {
				ui.Text(c, integrationCredits(credits.Total)).FontSize(design.TextSize(c, design.TextTitleLarge)).FontWeight(500)
				ui.Text(c, tr(c, "credits")).FontSize(design.TextSize(c, design.TextBase)).TextColor(a.palette.Muted).Margin(0, 0, 3, 5)
				ui.Spacer(c)
				label := integrationHours(credits.Total, a.state.Settings.General.Language)
				if credits.Total <= 0 {
					label = "Choose a plan or buy credits to run Cloud"
				}
				ui.Text(c, tr(c, label)).FontSize(design.TextSize(c, design.TextBase)).TextColor(a.palette.Muted).Margin(0, 0, 3, 0)
			})
			if credits.Monthly+credits.Extra > 0 {
				ui.Box(c).FillWidth().Height(4).Radius(2).Clip().Draw(func(p *ui.Painter, r ui.Rect) {
					fraction := float32(credits.Monthly / max(1, credits.Monthly+credits.Extra))
					p.Fill(ui.Rect{X: r.X, Y: r.Y, W: r.W * fraction, H: r.H}, a.palette.Text.Alpha(.7), 0)
					p.Fill(ui.Rect{X: r.X + r.W*fraction, Y: r.Y, W: r.W * (1 - fraction), H: r.H}, a.palette.Text.Alpha(.3), 0)
				})
			}
		})
		line(c)
		description, value := "No plan yet — choose one below", "—"
		if credits.MonthlyTotal > 0 {
			description = tr(c, "Of ") + integrationCredits(credits.MonthlyTotal)
			if !credits.MonthlyEndsAt.IsZero() {
				description += tr(c, " · resets ") + date(c, credits.MonthlyEndsAt.Local(), "Jan 2")
			}
			value = integrationCredits(credits.Monthly)
		}
		a.settingsRow(c, "Plan credits", description, func() { ui.Text(c, value).FontSize(design.TextSize(c, design.TextBase)) })
		line(c)
		a.settingsRow(c, "Extra credits", "Never expire · used once plan credits run out", func() { ui.Text(c, integrationCredits(credits.Extra)).FontSize(design.TextSize(c, design.TextBase)) })
	})
	sectionTitle(c, tr(c, "Plan"))
	ui.Row(c).FillWidth().Gap(12).Children(func() {
		for _, plan := range b.Plans {
			a.billingPlanCard(c, b, plan)
		}
	})
	if b.Subscription != nil {
		plan := contracts.BillingPlan{}
		for _, p := range b.Plans {
			if p.ID == b.Subscription.PlanID {
				plan = p
			}
		}
		detail := tr(c, "Renews ") + date(c, b.Subscription.PeriodEnd.Local(), "Jan 2") + "."
		if b.Subscription.CancelAtPeriodEnd {
			detail = tr(c, "Ends ") + date(c, b.Subscription.PeriodEnd.Local(), "Jan 2") + tr(c, ". You keep its credits until then.")
		}
		if b.Subscription.Status == "past_due" {
			detail = "Your last payment didn't go through — update your card to keep your plan."
		}
		a.settingsCard(c, func() {
			a.settingsRow(c, plan.Name+" · "+integrationPrice(plan.PriceCents)+tr(c, " a month"), detail, func() {
				textButton(c, tr(c, "Manage subscription")).Disabled(b.Checkout != nil && b.Checkout.State == "waiting").OnClick(func() { a.dispatch(contracts.Command{Kind: "billing.manage"}) })
			})
		})
	}
	sectionTitle(c, tr(c, "Extra credits"))
	if a.integrationUI.Packs < 1 {
		a.integrationUI.Packs = 1
	}
	packs := a.integrationUI.Packs
	a.settingsCard(c, func() {
		a.settingsRow(c, integrationCredits(b.Pack.Credits*float64(packs))+tr(c, " credits"), tr(c, "Never expire · ")+integrationHours(b.Pack.Credits*float64(packs), a.state.Settings.General.Language), func() {
			ui.Row(c).Height(32).Radius(8).Background(a.palette.Text.Alpha(.04)).AlignItems(ui.Center).Children(func() {
				iconButton(c, "minus", tr(c, "Fewer packs")).Disabled(packs <= 1).OnClick(func() { a.integrationUI.Packs = max(1, packs-1) })
				ui.Text(c, integrationCredits(float64(packs))).MinWidth(24).TextAlign(ui.Center).FontSize(design.TextSize(c, design.TextBase))
				iconButton(c, "plus", tr(c, "More packs")).Disabled(packs >= 20).OnClick(func() { a.integrationUI.Packs = min(20, packs+1) })
			})
			textButton(c, tr(c, "Buy for ")+tr(c, integrationPrice(b.Pack.PriceCents*packs))).MinWidth(96).Disabled(!b.Available || b.Checkout != nil && b.Checkout.State == "waiting").OnClick(func() {
				a.dispatch(contracts.Command{Kind: "billing.buy", Integration: &contracts.IntegrationRequest{Packs: packs}})
			})
		})
	})
}

func (a *App) billingPlanCard(c *ui.Context, b contracts.BillingState, plan contracts.BillingPlan) {
	current := b.Subscription != nil && b.Subscription.PlanID == plan.ID
	ui.Column(c.Key("billing-plan-"+plan.ID)).Grow(1).MinWidth(0).Padding(16).Gap(8).Radius(12).Background(a.palette.Card).Border(1, a.palette.Border).Children(func() {
		ui.Row(c).FillWidth().AlignItems(ui.Center).Children(func() {
			ui.Text(c, plan.Name).FontSize(design.TextSize(c, design.TextBase)).FontWeight(500)
			ui.Spacer(c)
			if current {
				ui.Text(c, tr(c, "Current")).FontSize(design.TextSize(c, design.TextXS)).TextColor(a.palette.Muted)
			}
		})
		ui.Row(c).AlignItems(ui.End).Gap(3).Children(func() {
			ui.Text(c, integrationPrice(plan.PriceCents)).FontSize(design.TextSize(c, design.TextTitle)).FontWeight(500)
			ui.Text(c, tr(c, "/ month")).FontSize(design.TextSize(c, design.TextSmall)).TextColor(a.palette.Muted)
		})
		ui.Text(c, tr(c, integrationCredits(plan.MonthlyCredits))+tr(c, " credits")).FontSize(design.TextSize(c, design.TextBase))
		ui.Text(c, integrationHours(plan.MonthlyCredits, a.state.Settings.General.Language)).FontSize(design.TextSize(c, design.TextBase)).TextColor(a.palette.Muted)
		label := tr(c, "Choose ") + plan.Name
		if current {
			label = "Current plan"
		} else if b.Subscription != nil {
			label = "Switch"
		}
		textButton(c, tr(c, label)).Label(tr(c, label) + " " + plan.Name).FillWidth().Disabled(!b.Available || current || b.Checkout != nil && b.Checkout.State == "waiting").OnClick(func() {
			a.dispatch(contracts.Command{Kind: "billing.subscribe", Integration: &contracts.IntegrationRequest{Value: plan.ID}})
		})
	})
}
