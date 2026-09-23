package mqtt

import (
	"reflect"
	"testing"
)

func TestExpandMeasureActions(t *testing.T) {
	tests := []struct {
		name    string
		sensors []string
		want    []string
	}{
		{name: "empty is all", sensors: nil, want: []string{"ph", "turb", "tds"}},
		{name: "all", sensors: []string{"all"}, want: []string{"ph", "turb", "tds"}},
		{name: "ui turbidity maps to turb", sensors: []string{"ph", "turbidity", "tds"}, want: []string{"ph", "turb", "tds"}},
		{name: "temp ignored", sensors: []string{"temp", "ph"}, want: []string{"ph"}},
		{name: "dedupe", sensors: []string{"ph", "ph", "turb"}, want: []string{"ph", "turb"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := expandMeasureActions(tt.sensors)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("got %v want %v", got, tt.want)
			}
		})
	}
}
