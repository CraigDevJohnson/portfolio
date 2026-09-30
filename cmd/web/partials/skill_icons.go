package partials

func conceptSkillIcon(name string) UIIconName {
	switch name {
	case "Cloud Architecture":
		return UIIconArchitecture
	case "Cloud Security", "Compliance & Governance", "Identity & Access Management", "Network Security", "Zero Trust Architecture":
		return UIIconSecurity
	case "DevSecOps", "Infrastructure Automation":
		return UIIconAutomation
	case "Observability", "Security Operations":
		return UIIconObservability
	case "Site Reliability Engineering":
		return UIIconInfrastructure
	default:
		return UIIconProblemSolving
	}
}
