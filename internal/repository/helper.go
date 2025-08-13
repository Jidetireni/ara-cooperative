package repository

type QueryType string

const (
	QueryTypeSelect QueryType = "select"
	QueryTypeCount  QueryType = "count"
)
