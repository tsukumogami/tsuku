package actions

// GetActionDeps returns the dependencies for an action by name.
// Returns an empty ActionDeps if the action is not found.
func GetActionDeps(actionName string) ActionDeps {
	act := Get(actionName)
	if act == nil {
		return ActionDeps{}
	}
	return act.Dependencies()
}
