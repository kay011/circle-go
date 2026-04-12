package planner

import (
	"testing"
	"time"
)

func TestGetNextTask_RespectsDependencies(t *testing.T) {
	p := NewPlanner(nil)
	now := time.Now()
	plan := &TaskPlan{
		Goal:   "g",
		Status: TaskPending,
		Tasks: []SubTask{
			{ID: "a", Description: "first", Status: TaskPending, Priority: PriorityMedium, CreatedAt: now},
			{ID: "b", Description: "second", Status: TaskPending, Priority: PriorityMedium, Dependencies: []string{"a"}, CreatedAt: now},
		},
	}

	next := p.GetNextTask(plan)
	if next == nil || next.ID != "a" {
		t.Fatalf("expected task a first, got %v", next)
	}

	p.UpdateTaskStatus(plan, "a", TaskCompleted, "ok", "")
	next = p.GetNextTask(plan)
	if next == nil || next.ID != "b" {
		t.Fatalf("expected task b after a completed, got %v", next)
	}
}

func TestUpdateTaskStatus_PlanAggregate(t *testing.T) {
	p := NewPlanner(nil)
	now := time.Now()
	plan := &TaskPlan{
		Goal:   "g",
		Status: TaskPending,
		Tasks: []SubTask{
			{ID: "x", Description: "one", Status: TaskPending, Priority: PriorityMedium, CreatedAt: now},
		},
	}
	p.UpdateTaskStatus(plan, "x", TaskCompleted, "r", "")
	if plan.Status != TaskCompleted {
		t.Errorf("plan status want completed, got %s", plan.Status)
	}
}

func TestParseTaskPlan_ValidJSON(t *testing.T) {
	raw := `{"goal":"ignored","tasks":[{"id":"t1","description":"d","status":"pending","priority":2}]}`
	plan, err := parseTaskPlan(raw, "user goal")
	if err != nil {
		t.Fatal(err)
	}
	if plan.Goal != "user goal" {
		t.Errorf("goal: %q", plan.Goal)
	}
	if len(plan.Tasks) != 1 || plan.Tasks[0].ID != "t1" {
		t.Fatalf("tasks: %+v", plan.Tasks)
	}
}
