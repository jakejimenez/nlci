package executor

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

// Confirm asks the user whether to run the already-displayed command.
// Returns true if the user confirms, false if they decline.
// The command has already been printed by Display(); this function only
// shows the prompt line to avoid double-printing.
func Confirm() bool {
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
