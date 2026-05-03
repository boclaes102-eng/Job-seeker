package scrapers

import (
	"math"
	"sort"
	"strings"
)

// BelgianCity is a city or large town in Flemish Belgium or Brussels,
// with coordinates for radius-based filtering.
type BelgianCity struct {
	Name string
	Lat  float64
	Lon  float64
}

// haversineKm returns the great-circle distance in kilometres between two points.
func haversineKm(aLat, aLon, bLat, bLon float64) float64 {
	const R = 6371.0
	dLat := (bLat - aLat) * math.Pi / 180
	dLon := (bLon - aLon) * math.Pi / 180
	lat1 := aLat * math.Pi / 180
	lat2 := bLat * math.Pi / 180
	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Sin(dLon/2)*math.Sin(dLon/2)*math.Cos(lat1)*math.Cos(lat2)
	return R * 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
}

// CitiesWithinRadius returns all Belgian cities within radiusKm of the named
// centre city, sorted nearest-first. If the centre city is not found in the
// list, all cities are returned so the caller always gets a non-empty result.
func CitiesWithinRadius(centerName string, radiusKm float64) []BelgianCity {
	var center *BelgianCity
	for i := range belgianCities {
		if strings.EqualFold(belgianCities[i].Name, centerName) {
			c := belgianCities[i]
			center = &c
			break
		}
	}
	if center == nil {
		return belgianCities
	}
	var result []BelgianCity
	for _, c := range belgianCities {
		if haversineKm(center.Lat, center.Lon, c.Lat, c.Lon) <= radiusKm {
			result = append(result, c)
		}
	}
	sort.Slice(result, func(i, j int) bool {
		di := haversineKm(center.Lat, center.Lon, result[i].Lat, result[i].Lon)
		dj := haversineKm(center.Lat, center.Lon, result[j].Lat, result[j].Lon)
		return di < dj
	})
	return result
}

// belgianCities covers every city and large town in the five Flemish provinces
// plus the Brussels-Capital Region.
var belgianCities = []BelgianCity{
	// Brussels-Capital Region
	{"Brussels", 50.8503, 4.3517},

	// Flemish Brabant
	{"Leuven", 50.8798, 4.7005},
	{"Vilvoorde", 50.9290, 4.4267},
	{"Aarschot", 50.9842, 4.8280},
	{"Tienen", 50.8064, 4.9420},
	{"Diest", 51.0003, 5.0529},
	{"Halle", 50.7348, 4.2360},
	{"Zaventem", 50.8872, 4.4726},
	{"Tervuren", 50.8221, 4.5319},
	{"Heist-op-den-Berg", 51.0693, 4.7204},
	{"Overijse", 50.7739, 4.5373},
	{"Haacht", 50.9741, 4.6342},
	{"Landen", 50.7534, 5.0881},
	{"Zoutleeuw", 50.8328, 5.1021},
	{"Tremelo", 51.0038, 4.7108},
	{"Rotselaar", 50.9607, 4.7154},
	{"Boortmeerbeek", 50.9778, 4.5748},

	// Province of Antwerp
	{"Antwerp", 51.2194, 4.4025},
	{"Mechelen", 51.0259, 4.4776},
	{"Turnhout", 51.3228, 4.9436},
	{"Geel", 51.1604, 4.9887},
	{"Herentals", 51.1800, 4.8333},
	{"Lier", 51.1310, 4.5699},
	{"Mol", 51.1857, 5.1167},
	{"Beveren", 51.2106, 4.2543},
	{"Sint-Niklaas", 51.1643, 4.1416},
	{"Westerlo", 51.0890, 4.9116},
	{"Olen", 51.1459, 4.8569},
	{"Retie", 51.2665, 5.0755},
	{"Zandhoven", 51.2205, 4.6598},
	{"Boom", 51.0894, 4.3686},
	{"Kontich", 51.1327, 4.4575},
	{"Willebroek", 51.0609, 4.3617},
	{"Bornem", 51.0988, 4.2397},
	{"Puurs-Sint-Amands", 51.0723, 4.2827},
	{"Aarschot", 50.9842, 4.8280}, // also in Flemish Brabant

	// East Flanders
	{"Ghent", 51.0543, 3.7174},
	{"Aalst", 50.9364, 4.0368},
	{"Dendermonde", 51.0289, 4.1012},
	{"Oudenaarde", 50.8417, 3.6078},
	{"Waregem", 50.8842, 3.4200},
	{"Lokeren", 51.1009, 3.9946},
	{"Wetteren", 50.9940, 3.8878},
	{"Geraardsbergen", 50.7706, 3.8813},
	{"Ninove", 50.8471, 3.9999},
	{"Zottegem", 50.8740, 3.8110},
	{"Ronse", 50.7454, 3.6013},
	{"Eeklo", 51.1847, 3.5700},
	{"Zelzate", 51.2027, 3.8099},
	{"Sint-Gillis-Waas", 51.2194, 4.1286},

	// West Flanders
	{"Bruges", 51.2093, 3.2240},
	{"Kortrijk", 50.8283, 3.2641},
	{"Roeselare", 50.9440, 3.1271},
	{"Ieper", 50.8509, 2.8826},
	{"Ostend", 51.2308, 2.9153},
	{"Veurne", 51.0703, 2.6625},
	{"Torhout", 51.0710, 3.1006},
	{"Tielt", 50.9950, 3.3256},
	{"Izegem", 50.9163, 3.2147},
	{"Poperinge", 50.8550, 2.7281},
	{"Menen", 50.8011, 3.1177},
	{"Harelbeke", 50.8588, 3.3111},
	{"Deerlijk", 50.8501, 3.3536},

	// Limburg
	{"Hasselt", 50.9307, 5.3378},
	{"Genk", 50.9650, 5.5012},
	{"Beringen", 51.0493, 5.2260},
	{"Tongeren", 50.7804, 5.4651},
	{"Maaseik", 51.1009, 5.7889},
	{"Lommel", 51.2340, 5.3126},
	{"Herk-de-Stad", 50.9457, 5.1641},
	{"Sint-Truiden", 50.8176, 5.1899},
	{"Bilzen", 50.8706, 5.5172},
	{"Bree", 51.1400, 5.5926},
	{"Neerpelt", 51.2269, 5.4372},
	{"Overpelt", 51.2040, 5.4194},
	{"Peer", 51.1322, 5.4567},
	{"Tessenderlo", 51.0659, 5.0891},
}
