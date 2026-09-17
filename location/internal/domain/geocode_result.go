package domain

// GeocodeResult is the resolved City/Country for a coordinate pair.
type GeocodeResult struct {
	City        string
	Country     string
	CountryCode string
}
