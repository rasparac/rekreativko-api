package domain

// ForwardGeocodeResult is the resolved coordinates for a forward-geocoding
// query, along with the City/Country Nominatim actually matched.
type ForwardGeocodeResult struct {
	Latitude    float64
	Longitude   float64
	DisplayName string
	City        string
	Country     string
	CountryCode string
}
