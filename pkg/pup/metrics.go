package pup

import (
	"fmt"
	"reflect"

	dogeboxd "github.com/dogeorg/dogeboxd/pkg"
)

// get all the metrics currently stored for a pup
func (t PupManager) GetMetrics(pupId string) map[string]interface{} {
	s, ok := t.stats[pupId]
	if !ok {
		fmt.Printf("Error: Unable to find stats for pup %s\n", pupId)
		return map[string]interface{}{}
	}

	metrics := make(map[string]interface{})
	for _, metric := range s.Metrics {
		metrics[metric.Name] = metric.Values.GetValues()
	}

	return metrics
}

// Updates the stats.Metrics field with data from the pup router
func (t PupManager) UpdateMetrics(u dogeboxd.UpdateMetrics) {
	s, ok := t.stats[u.PupID]
	if !ok {
		fmt.Println("skipping metrics for unfound pup", u.PupID)
		return
	}
	p := t.state[u.PupID]

	for _, m := range p.Manifest.Metrics {
		val, ok := u.Payload[m.Name]
		if !ok {
			// no value for metric
			continue
		}

		switch m.Type {
		case "string":
			v, ok := val.Value.(string)
			if !ok {
				fmt.Printf("metric value for %s is not string", m.Name)
				continue
			}
			t.addMetricValue(s, m.Name, v)
		case "int":
			// convert various things to int..
			var vi int
			switch v := val.Value.(type) {
			case float32:
				vi = int(v)
			case float64:
				vi = int(v)
			case int:
				vi = v
			default:
				fmt.Printf("metric value for %s is not int: %s", m.Name, reflect.TypeOf(val.Value))
				continue
			}
			t.addMetricValue(s, m.Name, vi)
		case "float":
			v, ok := val.Value.(float64)
			if !ok {
				fmt.Printf("metric value for %s is not float", m.Name)
				continue
			}
			t.addMetricValue(s, m.Name, v)
		default:
			fmt.Println("Manifest metric unknown field type", m.Type)
		}
	}
}

func (t PupManager) addMetricValue(stats *dogeboxd.PupStats, name string, value any) {
	for _, m := range stats.Metrics {
		if m.Name == name {
			m.Values.Add(value)
		}
	}
}
