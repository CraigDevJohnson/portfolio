package main

import (
	"crypto/md5"
	"encoding/hex"
	"html/template"
	"io"
	"log"
	"mime"
	"net/http"
	"strconv"
	"strings"
	"time"
)

/*
========================================
Template system
========================================
*/

var templateFuncs = template.FuncMap{
	"Year": func() int {
		return time.Now().Year()
	},
	"multiply": func(a, b int) int {
		return a * b
	},
	"slice": func(args ...int) []int {
		return args
	},
	"hasPrefix": func(s, prefix string) bool {
		return strings.HasPrefix(s, prefix)
	},
	"mod": func(a, b int) int {
		return a % b
	},
	"subtract": func(a, b int) int {
		return a - b
	},
}

var templatesByPage map[string]*template.Template

func loadTemplates() error {
	templatesByPage = make(map[string]*template.Template)

	// shared layout + partials
	base := "templates/layouts/base.html"
	header := "templates/partials/header.html"
	nav := "templates/partials/nav.html"
	footer := "templates/partials/footer.html"

	// page → page template
	pages := map[string]string{
		"home":       "templates/pages/home.html",
		"about":      "templates/pages/about.html",
		"experience": "templates/pages/experience.html",
		"skills":     "templates/pages/skills.html",
		"projects":   "templates/pages/projects.html",
		"education":  "templates/pages/education.html",
		"contact":    "templates/pages/contact.html",
		"soccer":     "templates/pages/soccer.html",
	}

	for page, pageTemplate := range pages {
		tmpl, err := template.New("base.html").
			Funcs(templateFuncs).
			ParseFiles(
				base,
				header,
				nav,
				footer,
				pageTemplate,
			)

		if err != nil {
			return err
		}

		// page-specific fragments
		switch page {
		case "soccer":
			if _, err := tmpl.ParseFiles(
				"templates/partials/soccer_table_fragment.html",
			); err != nil {
				return err
			}
		case "experience":
			if _, err := tmpl.ParseFiles(
				"templates/partials/experience_timeline.html",
			); err != nil {
				return err
			}
		case "skills":
			if _, err := tmpl.ParseFiles(
				"templates/partials/skills_grid.html",
			); err != nil {
				return err
			}
		case "projects":
			if _, err := tmpl.ParseFiles(
				"templates/partials/projects_grid.html",
			); err != nil {
				return err
			}
		}

		templatesByPage[page] = tmpl
	}

	return nil
}

func renderPage(w http.ResponseWriter, page string, data any) {
	tmpl := templatesByPage[page]
	if tmpl == nil {
		http.Error(w, "template not found", http.StatusInternalServerError)
		return
	}
	if err := tmpl.ExecuteTemplate(w, "base.html", data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func renderFragment(w http.ResponseWriter, page, name string, data any) {
	tmpl := templatesByPage[page]
	if tmpl == nil {
		http.Error(w, "template not found", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.ExecuteTemplate(w, name, data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

/*
========================================
Main
========================================
*/

func main() {
	// MIME types
	mime.AddExtensionType(".css", "text/css")
	mime.AddExtensionType(".js", "application/javascript")
	mime.AddExtensionType(".ico", "image/x-icon")
	mime.AddExtensionType(".svg", "image/svg+xml")
	mime.AddExtensionType(".webp", "image/webp")
	mime.AddExtensionType(".png", "image/png")
	mime.AddExtensionType(".jpg", "image/jpeg")

	// load templates ONCE at startup
	if err := loadTemplates(); err != nil {
		log.Fatal("template load failed:", err)
	}

	// routes - pages
	http.HandleFunc("/", homeHandler)
	http.HandleFunc("/about", aboutHandler)
	http.HandleFunc("/experience", experienceHandler)
	http.HandleFunc("/experience/timeline", experienceTimelineHandler)
	http.HandleFunc("/skills", skillsHandler)
	http.HandleFunc("/skills/grid", skillsGridHandler)
	http.HandleFunc("/projects", projectsHandler)
	http.HandleFunc("/projects/grid", projectsGridHandler)
	http.HandleFunc("/education", educationHandler)
	http.HandleFunc("/contact", contactHandler)

	// soccer routes
	http.HandleFunc("/soccer", soccerHandler)
	http.HandleFunc("/soccer/fetch", fetchSchedulesHandler)
	http.HandleFunc("/soccer/download", downloadICSHandler)
	http.HandleFunc("/soccer/subscribe", subscribeHandler)

	// static files
	http.Handle(
		"/static/",
		http.StripPrefix("/static/",
			http.FileServer(http.Dir("static")),
		),
	)

	http.HandleFunc("/favicon.ico", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "static/images/favicon.ico")
	})

	log.Println("Craig Johnson Portfolio running at http://localhost:8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}

/*
========================================
Home
========================================
*/

func gravatarURL(email string, size int) string {
	email = strings.TrimSpace(strings.ToLower(email))
	hash := md5.Sum([]byte(email))
	return "https://www.gravatar.com/avatar/" + hex.EncodeToString(hash[:]) + "?s=" + strconv.Itoa(size)
}

func homeHandler(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	renderPage(w, "home", map[string]any{
		"Title":       "Craig Johnson - System Engineer Advisor",
		"Page":        "home",
		"Name":        "Craig Johnson",
		"Role":        "System Engineer Advisor",
		"AvatarURL":   gravatarURL("gravatar@craigdevjohnson.com", 275),
		"Description": "Hi there! I'm a seasoned System Engineer with over a decade of experience in system engineering, administration, and optimization. I specialize in designing, implementing, and maintaining various systems and applications, thriving on performance optimization and security enhancement. I enjoy collaborating with application owners and software engineers to deliver innovative solutions and streamline processes through automation. I'm passionate about modernizing infrastructure and documenting critical processes. Let's connect and share our tech journeys!",
	})
}

/*
========================================
About
========================================
*/

func aboutHandler(w http.ResponseWriter, r *http.Request) {
	renderPage(w, "about", map[string]any{
		"Title": "About - Craig Johnson",
		"Page":  "about",
	})
}

/*
========================================
Experience
========================================
*/

type Experience struct {
	ID               int
	Position         string
	Company          string
	Duration         string
	Responsibilities string
	Technologies     []string
	SkillAreas       string
	Side             string
}

func experienceData() []Experience {
	return []Experience{
		{
			ID:               1,
			Position:         "System Engineer Advisor",
			Company:          "COMPANY REDACTED - A",
			Duration:         "2022 – Present",
			Responsibilities: "Lead infrastructure automation initiatives using IaC principles. Implement CI/CD pipelines for application deployment and configuration management. Architect and maintain cloud-native solutions while optimizing application performance and security. Develop self-service capabilities through automation, reducing deployment time by implementing GitOps methodologies.",
			Technologies:     []string{"Ansible", "Terraform", "AWS", "PowerShell"},
			SkillAreas:       "cloud,automation,devops,scripting,security",
			Side:             "left",
		},
		{
			ID:               2,
			Position:         "System Administrator",
			Company:          "COMPANY REDACTED - B",
			Duration:         "2021 – 2022",
			Responsibilities: "Managed enterprise SCADA systems and infrastructure automation. Implemented monitoring solutions and maintained high-availability environments. Established IT/OT integration practices while ensuring regulatory compliance. Orchestrated application deployments and infrastructure upgrades in critical environments.",
			Technologies:     []string{"IoT", "SCADA", "RHEL", "Bash"},
			SkillAreas:       "systems,automation,security,scripting",
			Side:             "right",
		},
		{
			ID:               3,
			Position:         "IT Systems Engineer Sr",
			Company:          "COMPANY REDACTED - C",
			Duration:         "2020 – 2021",
			Responsibilities: "Architected and implemented cloud infrastructure solutions in healthcare environments. Led technical projects involving cross-functional teams and vendor integration. Developed automation frameworks for critical systems and established best practices for infrastructure management.",
			Technologies:     []string{"Azure", "AD DS", "PowerShell"},
			SkillAreas:       "cloud,systems,automation,scripting",
			Side:             "left",
		},
		{
			ID:               4,
			Position:         "IT Systems Engineer",
			Company:          "COMPANY REDACTED - C",
			Duration:         "2018 – 2020",
			Responsibilities: "Managed enterprise Active Directory and Exchange infrastructure. Implemented automation solutions for service deployment and configuration management. Orchestrated application lifecycle management and infrastructure upgrades.",
			Technologies:     []string{"PowerShell", "AD DS", "O365/Exchange"},
			SkillAreas:       "systems,automation,scripting",
			Side:             "right",
		},
		{
			ID:               5,
			Position:         "IT Desktop Engineer",
			Company:          "COMPANY REDACTED - C",
			Duration:         "2017 – 2018",
			Responsibilities: "Implemented automated solutions for endpoint management and configuration. Managed incident response for business-critical systems using ITIL methodologies. Established standardized deployment procedures for enterprise endpoints.",
			Technologies:     []string{"PowerShell", "SCCM", "Intune"},
			SkillAreas:       "systems,automation,scripting",
			Side:             "left",
		},
		{
			ID:               6,
			Position:         "IT Service Desk Associate",
			Company:          "COMPANY REDACTED - C",
			Duration:         "2016 – 2017",
			Responsibilities: "Utilized ITSM platforms for incident and change management. Maintained documentation for standard operating procedures. Provided technical support for enterprise applications and systems.",
			Technologies:     []string{"ServiceNow", "O365", "Windows"},
			SkillAreas:       "systems",
			Side:             "right",
		},
		{
			ID:               7,
			Position:         "Service Desk Student Analyst",
			Company:          "COMPANY REDACTED - D",
			Duration:         "2012 – 2016",
			Responsibilities: "Managed incident tracking through enterprise ITSM systems. Maintained technical documentation and knowledge base articles. Achieved consistent high-quality metrics in service delivery.",
			Technologies:     []string{"Windows", "MacOS", "GoogleApps"},
			SkillAreas:       "systems",
			Side:             "left",
		},
	}
}

func experienceHandler(w http.ResponseWriter, r *http.Request) {
	renderPage(w, "experience", map[string]any{
		"Title":       "Experience - Craig Johnson",
		"Page":        "experience",
		"Experiences": experienceData(),
	})
}

func experienceTimelineHandler(w http.ResponseWriter, r *http.Request) {
	renderFragment(w, "experience", "experience_timeline.html", map[string]any{
		"Experiences": experienceData(),
	})
}

/*
========================================
Skills
========================================
*/

type Skill struct {
	ID   int
	Name string
	Icon string
	Link string
}

type SkillCategory struct {
	Name   string
	Skills []Skill
}

func skillsData() []SkillCategory {
	return []SkillCategory{
		{
			Name: "Cloud & Infrastructure",
			Skills: []Skill{
				{ID: 1, Name: "AWS Architecture", Link: "https://aws.amazon.com/about-aws/"},
				{ID: 2, Name: "Azure Architecture", Link: "https://azure.microsoft.com/en-us/explore"},
				{ID: 3, Name: "VMware ESXi/vSphere", Link: "https://www.vmware.com/"},
				{ID: 4, Name: "Infrastructure as Code", Link: "https://www.redhat.com/en/topics/automation/what-is-infrastructure-as-code-iac"},
				{ID: 5, Name: "Hybrid Cloud", Link: "https://www.ibm.com/topics/hybrid-cloud"},
			},
		},
		{
			Name: "DevOps & Automation",
			Skills: []Skill{
				{ID: 6, Name: "Ansible", Link: "https://www.ansible.com/overview/it-automation"},
				{ID: 7, Name: "Terraform", Link: "https://www.terraform.io/intro"},
				{ID: 8, Name: "Docker", Link: "https://www.docker.com/why-docker"},
				{ID: 9, Name: "Git/GitHub", Link: "https://github.com/about"},
				{ID: 10, Name: "CI/CD Pipelines", Link: "https://about.gitlab.com/topics/ci-cd/"},
			},
		},
		{
			Name: "Programming & Scripting",
			Skills: []Skill{
				{ID: 11, Name: "PowerShell", Link: "https://learn.microsoft.com/en-us/powershell/"},
				{ID: 12, Name: "Python", Link: "https://www.python.org/about/"},
				{ID: 13, Name: "Bash", Link: "https://www.gnu.org/software/bash/"},
				{ID: 14, Name: "JavaScript", Link: "https://www.w3schools.com/js/js_intro.asp"},
				{ID: 15, Name: "Go", Link: "https://go.dev/"},
			},
		},
		{
			Name: "Security & Compliance",
			Skills: []Skill{
				{ID: 16, Name: "Zero Trust Architecture", Link: "https://www.cloudflare.com/learning/security/glossary/what-is-zero-trust/"},
				{ID: 17, Name: "Identity & Access Management", Link: "https://www.gartner.com/en/information-technology/glossary/identity-and-access-management-iam"},
				{ID: 18, Name: "Cloud Security", Link: "https://www.checkpoint.com/cyber-hub/cloud-security/what-is-cloud-security/"},
				{ID: 19, Name: "Security Operations", Link: "https://www.cyberark.com/what-is-security-operations/"},
				{ID: 20, Name: "Compliance Frameworks", Link: "https://www.rapid7.com/fundamentals/compliance-regulatory-frameworks/"},
			},
		},
		{
			Name: "Systems & Services",
			Skills: []Skill{
				{ID: 21, Name: "Windows Server", Link: "https://www.microsoft.com/en-us/windows-server"},
				{ID: 22, Name: "Linux (RHEL)", Link: "https://www.redhat.com/en/technologies/linux-platforms/enterprise-linux"},
				{ID: 23, Name: "Load Balancers", Link: "https://www.f5.com/glossary/load-balancer"},
				{ID: 24, Name: "Configuration Management", Link: "https://www.atlassian.com/microservices/microservices-architecture/configuration-management"},
			},
		},
	}
}

func skillsHandler(w http.ResponseWriter, r *http.Request) {
	renderPage(w, "skills", map[string]any{
		"Title":      "Skills - Craig Johnson",
		"Page":       "skills",
		"Categories": skillsData(),
	})
}

func skillsGridHandler(w http.ResponseWriter, r *http.Request) {
	renderFragment(w, "skills", "skills_grid.html", map[string]any{
		"Categories": skillsData(),
	})
}

/*
========================================
Projects
========================================
*/

type Project struct {
	ID           int
	Name         string
	Intro        string
	Description  string
	Technologies []string
	Image        string
	GitHubURL    string
	DemoURL      string
	Category     string
}

func projectsData() []Project {
	return []Project{
		{
			ID:           1,
			Name:         "Personal Portfolio Website",
			Intro:        "A modern, responsive portfolio built with Go and HTMX",
			Description:  "Showcases my projects, skills, and certifications with a focus on cloud and web technologies.",
			Technologies: []string{"Go", "HTMX", "CSS", "HTML", "GitHub", "AWS"},
			Image:        "/static/images/projects/portfolio.webp",
			GitHubURL:    "https://github.com/CraigDevJohnson/craig-johnson-portfolio-vue",
			DemoURL:      "https://craigdevjohnson.com",
			Category:     "Web",
		},
		{
			ID:           2,
			Name:         "New User Account Provisioning",
			Intro:        "PowerShell scripts to fully automate user account creation and configuration.",
			Description:  "Completely automated new user account creation and configuration based on database push of new user information. This automation included creating the new user's active directory account, email account in O365/Exchange, and role based group memberships.",
			Technologies: []string{"PowerShell", "Git", "APIs", "AD DS", "O365/Exchange"},
			Image:        "/static/images/projects/provisioning.webp",
			Category:     "Automation",
		},
		{
			ID:           3,
			Name:         "Soccer Schedule Scraper",
			Intro:        "A web scraper to pull and parse team schedules and download as ICS file.",
			Description:  "A multi function Python script deployed as an AWS Lambda function to scrape and parse soccer team schedules and return them in ICS file format for broadly supported calendar importing.",
			Technologies: []string{"Python", "AWS Lambda", "GitHub", "API"},
			Image:        "/static/images/projects/scraper.webp",
			GitHubURL:    "https://github.com/CraigDevJohnson/soccer-scraper",
			DemoURL:      "/soccer",
			Category:     "Automation",
		},
	}
}

func projectsHandler(w http.ResponseWriter, r *http.Request) {
	renderPage(w, "projects", map[string]any{
		"Title":    "Projects - Craig Johnson",
		"Page":     "projects",
		"Projects": projectsData(),
	})
}

func projectsGridHandler(w http.ResponseWriter, r *http.Request) {
	renderFragment(w, "projects", "projects_grid.html", map[string]any{
		"Projects": projectsData(),
	})
}

/*
========================================
Education
========================================
*/

type Education struct {
	ID          int
	Degree      string
	Field       string
	Institution string
	Year        string
	Description string
}

func educationData() []Education {
	return []Education{
		{
			ID:          1,
			Degree:      "Bachelor of Science",
			Field:       "Computer Information & Office Systems",
			Institution: "University of Alaska Fairbanks",
			Year:        "2016",
			Description: "Focused on information systems, database management, and business applications of technology.",
		},
	}
}

func educationHandler(w http.ResponseWriter, r *http.Request) {
	renderPage(w, "education", map[string]any{
		"Title":     "Education - Craig Johnson",
		"Page":      "education",
		"Education": educationData(),
	})
}

/*
========================================
Contact
========================================
*/

func contactHandler(w http.ResponseWriter, r *http.Request) {
	renderPage(w, "contact", map[string]any{
		"Title": "Contact - Craig Johnson",
		"Page":  "contact",
	})
}

/*
========================================
Soccer
========================================
*/

type Game struct {
	ID       string `json:"id"`
	DateTime string `json:"datetime"`
	Field    string `json:"field"`
	Home     string `json:"home"`
	Away     string `json:"away"`
	Season   string `json:"season"`
}

type LambdaGamesResponse struct {
	Games []Game `json:"games"`
}

func soccerHandler(w http.ResponseWriter, r *http.Request) {
	renderPage(w, "soccer", map[string]any{
		"Title": "Soccer Schedule - Craig Johnson",
		"Page":  "soccer",
	})
}

func fetchSchedulesHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	_ = r.ParseForm()
	teamCodes := r.FormValue("team_codes")
	data := map[string]any{
		"TeamCodes": teamCodes,
		"Games":     mockFetchGames(parseTeamCodes(teamCodes)).Games,
	}
	renderFragment(w, "soccer", "soccer_table_fragment.html", data)
}

func subscribeHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	io.WriteString(w, `<div class="subscribe-success">✅ Subscribed! Check your email to confirm.</div>`)
}

/*
========================================
Mocks / helpers
========================================
*/

func mockFetchGames(teamCodes []string) LambdaGamesResponse {
	if len(teamCodes) == 0 {
		return LambdaGamesResponse{Games: []Game{}}
	}
	return LambdaGamesResponse{
		Games: []Game{
			{
				ID:       "sample1",
				DateTime: "Sun 01/11/26 02:55 PM",
				Field:    "3",
				Home:     "YOUR TEAM",
				Away:     "OPPONENT A",
				Season:   "168",
			},
			{
				ID:       "sample2",
				DateTime: "Sun 01/18/26 04:30 PM",
				Field:    "5",
				Home:     "OPPONENT B",
				Away:     "YOUR TEAM",
				Season:   "168",
			},
			{
				ID:       "sample3",
				DateTime: "Sun 01/25/26 01:00 PM",
				Field:    "2",
				Home:     "YOUR TEAM",
				Away:     "OPPONENT C",
				Season:   "168",
			},
		},
	}
}

func parseTeamCodes(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	return strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == ';' || r == ' '
	})
}

func downloadICSHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Generate a sample ICS file
	icsContent := `BEGIN:VCALENDAR
VERSION:2.0
PRODID:-//Craig Johnson Portfolio//Soccer Schedule//EN
BEGIN:VEVENT
UID:sample1@craigdevjohnson.com
DTSTART:20260111T145500
DTEND:20260111T165500
SUMMARY:Soccer: YOUR TEAM vs OPPONENT A
LOCATION:Field 3
DESCRIPTION:Season 168
END:VEVENT
END:VCALENDAR`

	w.Header().Set("Content-Type", "text/calendar")
	w.Header().Set("Content-Disposition", "attachment; filename=soccer_schedule.ics")
	io.WriteString(w, icsContent)
}
