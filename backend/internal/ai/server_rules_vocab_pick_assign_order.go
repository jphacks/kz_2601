package ai

import "strings"

// What fewest / most counts when records are saved one after another
// (runs after server_rules_vocab_pick_assign.go: files initialize in name
// order). The runner takes the events in order and finishes one before
// planning the next; saves that land at almost the same time may still be
// counted before each other's assignment.

// serverRulesAssignOrderLine: the assign section's line -> replacement.
var serverRulesAssignOrderLine = [2]string{
	`まとめて割り当てても 1 人に偏らない。`,
	`まとめて割り当てても 1 人に偏らない。保存のたびに動くルールも保存された順に 1 件ずつ動き、前の割り当てを数える(ほぼ同時の保存は同じ候補になることがある)。`,
}

func init() {
	l := serverRulesAssignOrderLine
	serverRulesAssignSection = strings.Replace(serverRulesAssignSection, l[0], l[1], 1)
	serverRulesReadsSection = strings.Replace(serverRulesReadsSection, l[0], l[1], 1) // it ends with the assign section
	serverRulesPrompt = strings.Replace(serverRulesPrompt, l[0], l[1], 1)
}
