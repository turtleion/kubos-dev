// Package main demonstrates how promptkit/selection is used.
package essentials

import (
	"github.com/erikgeiser/promptkit/selection"
)

func AskSelection(msg string, choices []string) (choice string, err error) {
	sp := selection.New(msg, choices)
	sp.PageSize = 3

	choice, err = sp.RunPrompt()
	if err != nil {
		return "", err
	}
	return choice, nil

}
