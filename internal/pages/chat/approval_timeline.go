package chat

func (entry *turn) linkedApprovals() map[string]int {
	var linked map[string]int
	for index := range entry.Clarifications {
		question := &entry.Clarifications[index]
		if question.Answer == "" {
			continue
		}
		id := entry.approvalCallID(question)
		if id == "" {
			continue
		}
		for _, activity := range entry.Activity {
			if activity.CallID == id {
				if linked == nil {
					linked = make(map[string]int)
				}
				linked[id] = index
				break
			}
		}
	}
	return linked
}

func (entry *turn) approvalCallID(question *clarification) string {
	approval := question.Approval
	if approval == nil {
		return ""
	}
	if approval.CallID != "" {
		return approval.CallID
	}
	// Older approvals did not record a call ID. Only link the preceding
	// activity when its arguments or shell command match the approval.
	if question.ActivityOffset == nil || approval.Preview == "" {
		return ""
	}
	offset := *question.ActivityOffset
	if offset < 1 || offset > len(entry.Activity) {
		return ""
	}
	activity := entry.Activity[offset-1]
	if activity.Arguments == approval.Preview || (activity.RunsCommand() && activity.Target == approval.Preview) {
		return activity.CallID
	}
	return ""
}
