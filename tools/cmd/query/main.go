package main

import (
	"fmt"
	"log"
	"strings"

	"github.com/spf13/cobra"
	"github.com/lithammer/fuzzysearch/fuzzy"
	"github.com/kljensen/snowball"
)

// Mock word embeddings model - replace with actual implementation
type WordEmbeddingsModel struct{}

func (w *WordEmbeddingsModel) FindSimilarWords(word string, n int) []string {
	// Mock implementation
	similarWords := map[string][]string{
		"tshirt": {"teeshirt", "t-shirt", "tee"},
		"jeans":  {"denim", "trousers", "pants"},
	}
	if words, ok := similarWords[word]; ok {
		return words[:min(n, len(words))]
	}
	return []string{}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

var rootCmd = &cobra.Command{
	Use:   "smartsearch [search term]",
	Short: "Perform a smart search on product names",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		query := args[0]
		products := []string{"T-shirt", "Jeans", "Sneakers", "Hoodie", "Tee Shirt", "T Shirt"}
		results := smartSearch(query, products)
		fmt.Printf("Search results for '%s':\n", query)
		for _, result := range results {
			fmt.Println("-", result)
		}
	},
}

func stemWord(word string) string {
	stemmed, err := snowball.Stem(word, "english", true)
	if err != nil {
		return word // Return original word if stemming fails
	}
	return stemmed
}

func fuzzySearch(query string, items []string, threshold int) []string {
	var matches []string
	for _, item := range items {
		if fuzzy.LevenshteinDistance(query, item) <= threshold {
			matches = append(matches, item)
		}
	}
	return matches
}

func uniqueMatches(matches []string) []string {
	seen := make(map[string]bool)
	var unique []string
	for _, match := range matches {
		if !seen[match] {
			seen[match] = true
			unique = append(unique, match)
		}
	}
	return unique
}

func smartSearch(query string, products []string) []string {
	stemmedQuery := stemWord(strings.ToLower(query))
	var matches []string

	// First, try exact match on stemmed words
	for _, product := range products {
		if stemWord(strings.ToLower(product)) == stemmedQuery {
			matches = append(matches, product)
		}
	}

	// If no exact matches, try fuzzy matching
	if len(matches) == 0 {
		matches = fuzzySearch(stemmedQuery, products, 3)
	}

	// If still no matches, try word embeddings
	if len(matches) == 0 {
		wordEmbeddingsModel := &WordEmbeddingsModel{}
		similarWords := wordEmbeddingsModel.FindSimilarWords(query, 5)
		for _, word := range similarWords {
			matches = append(matches, fuzzySearch(word, products, 3)...)
		}
	}

	return uniqueMatches(matches)
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		log.Fatal(err)
	}
}
