package contracts

import "testing"

func TestSnapshotOwnsDurableStatisticsAndScopesTurnDetails(t *testing.T) {
	source := Snapshot{
		Sessions: []Session{{ID: "main"}, {ID: "child"}},
		Metrics: map[string]ConversationMetrics{
			"main":  {Total: ExecutionMetrics{Models: []ModelMetrics{{Provider: "saved"}}}, Turns: map[string]ExecutionMetrics{"run": {Turns: 1}}},
			"child": {Own: ExecutionMetrics{WallKnown: true}, Turns: map[string]ExecutionMetrics{"child-run": {Turns: 1}}},
		},
		ApplicationUsage: &ApplicationUsage{Report: ApplicationUsageReport{Days: map[string]ApplicationUsageDay{"day": {Models: map[string]int64{"model": 10}}}}},
	}
	copied := CloneSnapshot(source)
	value := copied.Metrics["main"]
	value.Total.Models[0].Provider = "edited"
	delete(value.Turns, "run")
	copied.ApplicationUsage.Report.Days["day"].Models["model"] = 20
	if source.Metrics["main"].Total.Models[0].Provider != "saved" || len(source.Metrics["main"].Turns) != 1 || source.ApplicationUsage.Report.Days["day"].Models["model"] != 10 {
		t.Fatal("snapshot statistics alias core data")
	}
	projection := SessionProjection(source, []string{"main"})
	if len(projection.Metrics["main"].Turns) != 1 || len(projection.Metrics["child"].Turns) != 0 || !projection.Metrics["child"].Own.WallKnown {
		t.Fatal("projection lost aggregate or leaked unrequested turn detail")
	}
	if projection.ApplicationUsage != nil {
		t.Fatal("conversation projection broadcast the application usage ledger")
	}
}
