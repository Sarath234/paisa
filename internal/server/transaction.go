package server

import (
	"sort"

	"github.com/ananthakumaran/paisa/internal/accounting"
	"github.com/ananthakumaran/paisa/internal/model/posting"
	"github.com/ananthakumaran/paisa/internal/model/transaction"
	"github.com/ananthakumaran/paisa/internal/query"
	"github.com/gin-gonic/gin"

	"gorm.io/gorm"
)

func GetTransactions(db *gorm.DB) gin.H {
	postings := query.Init(db).Desc().All()
	transactions := transaction.Build(postings)

	sort.Slice(transactions, func(i, j int) bool { return transactions[i].ID > transactions[j].ID })
	sort.SliceStable(transactions, func(i, j int) bool { return transactions[i].Date.After(transactions[j].Date) })

	return gin.H{"transactions": transactions}
}

func GetBalancedPostings(db *gorm.DB) gin.H {
	postings := query.Init(db).Desc().All()
	transactions := transaction.Build(postings)
	balancePostings := accounting.BuildBalancedPostings(transactions)

	return gin.H{"balancedPostings": balancePostings}
}

func GetLatestTransactions(allPostings []posting.Posting) []transaction.Transaction {
	transactions := transaction.Build(allPostings)

	sort.Slice(transactions, func(i, j int) bool { return transactions[i].ID > transactions[j].ID })
	sort.SliceStable(transactions, func(i, j int) bool { return transactions[i].Date.After(transactions[j].Date) })

	if len(transactions) > 200 {
		transactions = transactions[:200]
	}
	return transactions
}
