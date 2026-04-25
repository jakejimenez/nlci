package executor

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

// Confirm presents the generated command and explanation to the user and
// asks for confirmation before execution.
// Returns true if the user confirms, false if they decline.
func Confirm(command, explanation string) bool {
	fmt.Printf("\n  > %s\n", command)
	if explanation != "" {
		fmt.Printf("    %s\n", explanation)
	}
	fmt.Print("\nRun this command? [y/N] ")

	reader := bufio.NewReader(os.Stdin)
	input, err := reader.ReadString('\n')
	if err != nil {
		return false
	}

	input = strings.TrimSpace(strings.ToLower(input))
	return input == "y" || input == "yes"
}

// Display prints the generated command and explanation without prompting.
func Display(command, explanation string) {
	fmt.Printf("\n  > %s\n", command)
	if explanation != "" {
		fmt.Printf("    %s\n", explanation)
	}
}
