package routing

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

const osrmBaseURL = "https://router.project-osrm.org/route/v1/driving"

type Point struct {
	Lat float64 `json:"lat"`
	Lng float64 `json:"lng"`
}

type Route struct {
	Points   []Point `json:"points"`
	Distance float64 `json:"distance"` // metros
	Duration float64 `json:"duration"` // segundos
}

type osrmResponse struct {
	Code   string `json:"code"`
	Routes []struct {
		Distance float64 `json:"distance"`
		Duration float64 `json:"duration"`
		Geometry struct {
			Coordinates [][2]float64 `json:"coordinates"`
		} `json:"geometry"`
	} `json:"routes"`
}

var client = &http.Client{Timeout: 10 * time.Second}

func GetRoute(stops []Point) (*Route, error) {
	if len(stops) < 2 {
		return nil, fmt.Errorf("at least 2 stops required")
	}

	coords := ""
	for i, s := range stops {
		if i > 0 {
			coords += ";"
		}
		coords += fmt.Sprintf("%f,%f", s.Lng, s.Lat)
	}

	url := fmt.Sprintf("%s/%s?overview=full&geometries=geojson&steps=false", osrmBaseURL, coords)

	resp, err := client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("osrm request: %w", err)
	}
	defer resp.Body.Close()

	var result osrmResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("osrm decode: %w", err)
	}

	if result.Code != "Ok" || len(result.Routes) == 0 {
		return nil, fmt.Errorf("osrm returned no route: %s", result.Code)
	}

	r := result.Routes[0]
	points := make([]Point, len(r.Geometry.Coordinates))
	for i, c := range r.Geometry.Coordinates {
		points[i] = Point{Lat: c[1], Lng: c[0]}
	}

	return &Route{
		Points:   points,
		Distance: r.Distance,
		Duration: r.Duration,
	}, nil
}
