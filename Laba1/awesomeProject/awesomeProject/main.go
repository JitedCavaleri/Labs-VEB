package main

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

func daysBeforeNewYear() int {
	now := time.Now()

	// следующий новый год
	nextYear := now.Year() + 1
	newYear := time.Date(nextYear, 1, 1, 0, 0, 0, 0, now.Location())

	// разница в днях
	diff := newYear.Sub(now)
	days := int(diff.Hours() / 24)

	return days
}

func main() {
	router := gin.Default()

	router.GET("/info", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"days_before_new_year": daysBeforeNewYear(),
		})
	})

	router.Run(":4200")
}
