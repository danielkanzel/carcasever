package game

// Base-game shapes. Edges are north, east, south, west.
// Corners of a field are NE=0, SE=1, SW=2, NW=3.
// Weights follow the classic 72-tile mix; the draw is with replacement.

const (
	City  = "city"
	Road  = "road"
	Field = "field"

	KindCity      = "city"
	KindRoad      = "road"
	KindField     = "field"
	KindMonastery = "monastery"
)

type TileDef struct {
	ID        string    `json:"id"`
	Weight    int       `json:"weight"`
	Edges     [4]string `json:"edges"`
	Cities    [][]int   `json:"cities"`
	Roads     [][]int   `json:"roads"`
	Fields    [][]int   `json:"fields"`
	Monastery bool      `json:"monastery"`
	Shield    bool      `json:"shield"`
}

var Catalog = []TileDef{
	{ID: "mon_road", Weight: 2, Monastery: true, Edges: [4]string{Field, Field, Road, Field}, Roads: [][]int{{2}}, Fields: [][]int{{0, 1, 2, 3}}},
	{ID: "mon", Weight: 4, Monastery: true, Edges: [4]string{Field, Field, Field, Field}, Fields: [][]int{{0, 1, 2, 3}}},
	{ID: "city4", Weight: 1, Shield: true, Edges: [4]string{City, City, City, City}, Cities: [][]int{{0, 1, 2, 3}}},
	{ID: "city_road", Weight: 4, Edges: [4]string{City, Road, Field, Road}, Cities: [][]int{{0}}, Roads: [][]int{{1, 3}}, Fields: [][]int{{0, 3}, {1, 2}}},
	{ID: "city1", Weight: 5, Edges: [4]string{City, Field, Field, Field}, Cities: [][]int{{0}}, Fields: [][]int{{0, 1, 2, 3}}},
	{ID: "bridge_s", Weight: 2, Shield: true, Edges: [4]string{City, Field, City, Field}, Cities: [][]int{{0, 2}}, Fields: [][]int{{0, 1}, {2, 3}}},
	{ID: "bridge", Weight: 1, Edges: [4]string{City, Field, City, Field}, Cities: [][]int{{0, 2}}, Fields: [][]int{{0, 1}, {2, 3}}},
	{ID: "opp", Weight: 3, Edges: [4]string{City, Field, City, Field}, Cities: [][]int{{0}, {2}}, Fields: [][]int{{0, 1, 2, 3}}},
	{ID: "corner", Weight: 2, Edges: [4]string{City, City, Field, Field}, Cities: [][]int{{0}, {1}}, Fields: [][]int{{0, 1, 2, 3}}},
	{ID: "bend_r", Weight: 3, Edges: [4]string{City, Road, Road, Field}, Cities: [][]int{{0}}, Roads: [][]int{{1, 2}}, Fields: [][]int{{0, 2, 3}, {1}}},
	{ID: "bend_l", Weight: 3, Edges: [4]string{City, Field, Road, Road}, Cities: [][]int{{0}}, Roads: [][]int{{2, 3}}, Fields: [][]int{{0, 1, 3}, {2}}},
	{ID: "tee_city", Weight: 3, Edges: [4]string{City, Road, Road, Road}, Cities: [][]int{{0}}, Roads: [][]int{{1}, {2}, {3}}, Fields: [][]int{{0, 3}, {1}, {2}}},
	{ID: "city2_s", Weight: 2, Shield: true, Edges: [4]string{City, City, Field, Field}, Cities: [][]int{{0, 1}}, Fields: [][]int{{1, 2, 3}}},
	{ID: "city2", Weight: 3, Edges: [4]string{City, City, Field, Field}, Cities: [][]int{{0, 1}}, Fields: [][]int{{1, 2, 3}}},
	{ID: "city2_road_s", Weight: 2, Shield: true, Edges: [4]string{City, Road, Road, City}, Cities: [][]int{{0, 3}}, Roads: [][]int{{1, 2}}, Fields: [][]int{{0, 2}, {1}}},
	{ID: "city2_road", Weight: 3, Edges: [4]string{City, Road, Road, City}, Cities: [][]int{{0, 3}}, Roads: [][]int{{1, 2}}, Fields: [][]int{{0, 2}, {1}}},
	{ID: "city3_s", Weight: 1, Shield: true, Edges: [4]string{City, City, Field, City}, Cities: [][]int{{0, 1, 3}}, Fields: [][]int{{1, 2}}},
	{ID: "city3", Weight: 3, Edges: [4]string{City, City, Field, City}, Cities: [][]int{{0, 1, 3}}, Fields: [][]int{{1, 2}}},
	{ID: "city3_road_s", Weight: 2, Shield: true, Edges: [4]string{City, City, Road, City}, Cities: [][]int{{0, 1, 3}}, Roads: [][]int{{2}}, Fields: [][]int{{1}, {2}}},
	{ID: "city3_road", Weight: 1, Edges: [4]string{City, City, Road, City}, Cities: [][]int{{0, 1, 3}}, Roads: [][]int{{2}}, Fields: [][]int{{1}, {2}}},
	{ID: "road", Weight: 8, Edges: [4]string{Road, Field, Road, Field}, Roads: [][]int{{0, 2}}, Fields: [][]int{{0, 1}, {2, 3}}},
	{ID: "bend", Weight: 9, Edges: [4]string{Road, Road, Field, Field}, Roads: [][]int{{0, 1}}, Fields: [][]int{{0}, {1, 2, 3}}},
	{ID: "tee", Weight: 4, Edges: [4]string{Field, Road, Road, Road}, Roads: [][]int{{1}, {2}, {3}}, Fields: [][]int{{0, 3}, {1}, {2}}},
	{ID: "cross", Weight: 1, Edges: [4]string{Road, Road, Road, Road}, Roads: [][]int{{0}, {1}, {2}, {3}}, Fields: [][]int{{0}, {1}, {2}, {3}}},
}

const StartTileID = "city_road"

func Def(id string) (TileDef, bool) {
	for _, d := range Catalog {
		if d.ID == id {
			return d, true
		}
	}
	return TileDef{}, false
}

func (d TileDef) View(rot int) View {
	rot = ((rot % 4) + 4) % 4
	v := View{ID: d.ID, Monastery: d.Monastery, Shield: d.Shield}
	for i := 0; i < 4; i++ {
		v.Edges[(i+rot)%4] = d.Edges[i]
	}
	v.Cities = turnGroups(d.Cities, rot)
	v.Roads = turnGroups(d.Roads, rot)
	v.Fields = turnGroups(d.Fields, rot)
	return v
}

func turnGroups(groups [][]int, rot int) [][]int {
	if len(groups) == 0 {
		return nil
	}
	out := make([][]int, len(groups))
	for i, g := range groups {
		out[i] = make([]int, len(g))
		for j, v := range g {
			out[i][j] = (v + rot) % 4
		}
	}
	return out
}

type View struct {
	ID        string
	Edges     [4]string
	Cities    [][]int
	Roads     [][]int
	Fields    [][]int
	Monastery bool
	Shield    bool
}
