package main

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"io"
	"log"
	"mime"
	"net/http"
	"portfolio/components/pages"
	"portfolio/components/partials"
	"portfolio/types"
	"strconv"
	"strings"
)

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

	// routes - pages
	http.HandleFunc("/", homeHandler)
	http.HandleFunc("/about", aboutHandler)
	http.HandleFunc("/experience", experienceHandler)
	http.HandleFunc("/experience/timeline", experienceTimelineHandler)
	http.HandleFunc("/skills", skillsHandler)
	http.HandleFunc("/skills/grid", skillsGridHandler)
	http.HandleFunc("/skills/filtered", skillsFilteredHandler)
	http.HandleFunc("/skills/detail", skillsDetailHandler)
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
	err := pages.Home(pages.HomeProps{
		Name:        "Craig Johnson",
		Role:        "Cloud Engineer Principal",
		AvatarURL:   gravatarURL("gravatar@craigdevjohnson.com", 275),
		Description: "Hi there! I'm a seasoned System Engineer with over a decade of experience in system engineering, administration, and optimization. I specialize in designing, implementing, and maintaining various systems and applications, thriving on performance optimization and security enhancement. I enjoy collaborating with application owners and software engineers to deliver innovative solutions and streamline processes through automation. I'm passionate about modernizing infrastructure and documenting critical processes. Let's connect and share our tech journeys!",
	}).Render(context.Background(), w)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

/*
========================================
About
========================================
*/

func aboutHandler(w http.ResponseWriter, r *http.Request) {
	err := pages.About().Render(context.Background(), w)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

/*
========================================
Experience
========================================
*/

// Use types from shared package
type Experience = types.Experience

func experienceData() []Experience {
	return []Experience{
		{
			ID:               1,
			Position:         "Cloud Engineer Principal",
			Company:          "COMPANY REDACTED - A",
			Duration:         "2022 – Present",
			Responsibilities: "Lead infrastructure automation initiatives using IaC principles. Implement CI/CD pipelines for application deployment and configuration management. Architect and maintain cloud-native solutions while optimizing application performance and security. Develop self-service capabilities through automation, reducing deployment time by implementing GitOps methodologies.",
			Technologies:     []string{"AWS", "Go", "Terraform", "Ansible"},
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
	err := pages.Experience().Render(context.Background(), w)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func experienceTimelineHandler(w http.ResponseWriter, r *http.Request) {
	props := partials.ExperienceTimelineProps{
		Experiences: experienceData(),
	}
	err := partials.ExperienceTimeline(props).Render(context.Background(), w)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

/*
========================================
Skills
========================================
*/

// Use types from shared package
type Skill = types.Skill
type SkillCategory = types.SkillCategory

const (
	iconZeroTrust      string = `<svg viewBox="0 0 24 24" fill="#8B5CF6" aria-hidden="true"><path d="M12 1l9 4v6c0 5.25-3.81 10.14-9 11-5.19-.86-9-5.75-9-11V5l9-4zm0 2.18L5 6.3v4.7c0 4.08 2.96 7.88 7 8.62 4.04-.74 7-4.54 7-8.62V6.3l-7-3.12zM12 7a2 2 0 110 4 2 2 0 010-4zm0 5c2.67 0 8 1.34 8 4v1H4v-1c0-2.66 5.33-4 8-4z"/></svg>`
	iconIdentityAccess string = `<svg viewBox="0 0 24 24" fill="#F59E0B" aria-hidden="true"><path d="M18.685 19.097A9.723 9.723 0 0021.75 12c0-5.385-4.365-9.75-9.75-9.75S2.25 6.615 2.25 12a9.723 9.723 0 003.065 7.097A9.716 9.716 0 0012 21.75a9.716 9.716 0 006.685-2.653zm-12.54-1.285A7.486 7.486 0 0112 15a7.486 7.486 0 015.855 2.812A8.224 8.224 0 0112 20.25a8.224 8.224 0 01-5.855-2.438zM15.75 9a3.75 3.75 0 11-7.5 0 3.75 3.75 0 017.5 0z"/></svg>`
	iconCloudSecurity  string = `<svg viewBox="0 0 24 24" fill="#EF4444" aria-hidden="true"><path d="M4.5 9.75a6 6 0 0111.573-2.226 3.75 3.75 0 014.133 4.303A4.5 4.5 0 0118 20.25H6.75a5.25 5.25 0 01-2.23-10.004 6.072 6.072 0 01-.02-.496z"/><path fill="#fff" d="M12 8l3 3h-2v3h-2v-3H9l3-3z"/></svg>`
	iconCompliance     string = `<svg viewBox="0 0 24 24" fill="#22C55E" aria-hidden="true"><path d="M9 12l2 2 4-4m6 2a9 9 0 11-18 0 9 9 0 0118 0z"/></svg>`
	iconMonitoring     string = `<svg viewBox="0 0 24 24" fill="#06B6D4" aria-hidden="true"><path d="M3 13h2v8H3v-8zm6-6h2v14H9V7zm6-4h2v18h-2V3zm6 8h2v10h-2V11z"/></svg>`
	iconInfraAuto      string = `<svg viewBox="0 0 24 24" fill="#A855F7" aria-hidden="true"><path d="M4 6h16v2H4V6zm0 5h16v2H4v-2zm0 5h16v2H4v-2z"/><path d="M18 9l3 3-3 3M6 9l-3 3 3 3" stroke="#A855F7" stroke-width="1.5" fill="none"/></svg>`
	iconCloudArch      string = `<svg viewBox="0 0 24 24" fill="#0EA5E9" aria-hidden="true"><path d="M4.5 9.75a6 6 0 0111.573-2.226 3.75 3.75 0 014.133 4.303A4.5 4.5 0 0118 20.25H6.75a5.25 5.25 0 01-2.23-10.004 6.072 6.072 0 01-.02-.496z"/></svg>`
	iconNetworkSec     string = `<svg viewBox="0 0 24 24" fill="#EC4899" aria-hidden="true"><path d="M12 2C6.48 2 2 6.48 2 12s4.48 10 10 10 10-4.48 10-10S17.52 2 12 2zm-1 17.93c-3.95-.49-7-3.85-7-7.93 0-.62.08-1.21.21-1.79L9 15v1c0 1.1.9 2 2 2v1.93zm6.9-2.54c-.26-.81-1-1.39-1.9-1.39h-1v-3c0-.55-.45-1-1-1H8v-2h2c.55 0 1-.45 1-1V7h2c1.1 0 2-.9 2-2v-.41c2.93 1.19 5 4.06 5 7.41 0 2.08-.8 3.97-2.1 5.39z"/></svg>`
	iconDevSecOps      string = `<svg viewBox="0 0 24 24" fill="#10B981" aria-hidden="true"><path d="M12 2L2 7l10 5 10-5-10-5zM2 17l10 5 10-5M2 12l10 5 10-5"/></svg>`
	iconSRE            string = `<svg viewBox="0 0 24 24" fill="#F97316" aria-hidden="true"><path d="M12 2v4m0 12v4M4.93 4.93l2.83 2.83m8.48 8.48l2.83 2.83M2 12h4m12 0h4M4.93 19.07l2.83-2.83m8.48-8.48l2.83-2.83"/><circle cx="12" cy="12" r="4" fill="#F97316"/></svg>`
	iconSecOps         string = `<svg viewBox="0 0 24 24" fill="#6366F1" aria-hidden="true"><path d="M12 2.25c-5.385 0-9.75 4.365-9.75 9.75s4.365 9.75 9.75 9.75 9.75-4.365 9.75-9.75S17.385 2.25 12 2.25zM12.75 6a.75.75 0 00-1.5 0v6c0 .414.336.75.75.75h4.5a.75.75 0 000-1.5h-3.75V6z"/></svg>`
)

func skillsData() []SkillCategory {
	return []SkillCategory{
		{
			Name: "Languages & Scripting",
			Skills: []Skill{
				{ID: 5, Name: "Bash", IconPath: "/static/images/skills/bash.svg", Link: "https://www.gnu.org/software/bash/", Proficiency: "expert", Featured: true, Description: "Unix shell and command language for task automation and system administration"},
				{ID: 2, Name: "Go", IconPath: "/static/images/skills/go.svg", Link: "https://go.dev/", Proficiency: "advanced", Featured: true, Description: "Statically typed language for building scalable cloud services and CLI tools"},
				{ID: 3, Name: "JavaScript", IconPath: "/static/images/skills/javascript.svg", Link: "https://developer.mozilla.org/en-US/docs/Web/JavaScript", Proficiency: "advanced"},
				{ID: 10, Name: "JSON", IconPath: "/static/images/skills/json.svg", Link: "https://www.json.org/", Proficiency: "expert"},
				{ID: 11, Name: "Markdown", IconPath: "/static/images/skills/markdown.svg", Link: "https://www.markdownguide.org/", Proficiency: "expert"},
				{ID: 6, Name: "PowerShell", IconPath: "/static/images/skills/powershell.svg", Link: "https://learn.microsoft.com/en-us/powershell/", Proficiency: "expert", Featured: true, Description: "Cross-platform framework for configuration management and task automation"},
				{ID: 1, Name: "Python", IconPath: "/static/images/skills/python.svg", Link: "https://www.python.org/", Proficiency: "expert", Featured: true, Description: "Versatile language for automation, scripting, and cloud infrastructure tooling"},
				{ID: 4, Name: "TypeScript", IconPath: "/static/images/skills/typescript.svg", Link: "https://www.typescriptlang.org/", Proficiency: "intermediate"},
				{ID: 9, Name: "YAML", IconPath: "/static/images/skills/yaml.svg", Link: "https://yaml.org/", Proficiency: "expert"},
			},
		},
		{
			Name: "Cloud Platforms",
			Skills: []Skill{
				{ID: 12, Name: "AWS", IconPath: "/static/images/skills/aws.svg", Link: "https://aws.amazon.com/", Proficiency: "expert", Featured: true, Description: "Primary cloud platform for compute, storage, networking, and serverless solutions"},
				{ID: 13, Name: "Azure", IconPath: "/static/images/skills/azure.svg", Link: "https://azure.microsoft.com/", Proficiency: "advanced", Featured: true, Description: "Microsoft cloud platform for hybrid identity, VMs, and enterprise services"},
				{ID: 15, Name: "Cloudflare", IconPath: "/static/images/skills/cloudflare.svg", Link: "https://www.cloudflare.com/", Proficiency: "intermediate"},
				{ID: 17, Name: "vSphere", IconPath: "/static/images/skills/vsphere.svg", Link: "https://www.vmware.com/products/vsphere.html", Proficiency: "advanced"},
			},
		},
		{
			Name: "Security & Identity",
			Skills: []Skill{
				{ID: 121, Name: "Cognito", IconPath: "/static/images/skills/aws_cognito.svg", Link: "https://aws.amazon.com/cognito/", Proficiency: "advanced"},
				{ID: 120, Name: "IAM", IconPath: "/static/images/skills/aws_iam.svg", Link: "https://aws.amazon.com/iam/", Proficiency: "expert", Featured: true, Description: "Identity and access management for implementing least-privilege security"},
				{ID: 30, Name: "Vault", IconPath: "/static/images/skills/hashicorp_vault.svg", Link: "https://www.vaultproject.io/", Proficiency: "advanced"},
			},
		},
		{
			Name: "Containers & Orchestration",
			Skills: []Skill{
				{ID: 18, Name: "Docker", IconPath: "/static/images/skills/docker.svg", Link: "https://www.docker.com/", Proficiency: "expert", Featured: true, Description: "Container platform for building, shipping, and running applications consistently"},
				{ID: 19, Name: "Kubernetes", IconPath: "/static/images/skills/kubernetes.svg", Link: "https://kubernetes.io/", Proficiency: "advanced", Featured: true, Description: "Container orchestration for deploying and scaling containerized workloads"},
				{ID: 20, Name: "Podman", IconPath: "/static/images/skills/podman.svg", Link: "https://podman.io/", Proficiency: "advanced", Featured: true, Description: "Daemonless container engine for running OCI containers and pods"},
				{ID: 101, Name: "Rancher", IconPath: "/static/images/skills/rancher.svg", Link: "https://www.rancher.com/", Proficiency: "intermediate"},
			},
		},
		{
			Name: "CI/CD & Automation",
			Skills: []Skill{
				{ID: 27, Name: "Ansible", IconPath: "/static/images/skills/ansible.svg", Link: "https://www.ansible.com/", Proficiency: "expert", Featured: true, Description: "Agentless automation for configuration management and application deployment"},
				{ID: 125, Name: "CodeBuild", IconPath: "/static/images/skills/aws_codebuild.svg", Link: "https://aws.amazon.com/codebuild/", Proficiency: "advanced"},
				{ID: 126, Name: "CodeDeploy", IconPath: "/static/images/skills/aws_codedeploy.svg", Link: "https://aws.amazon.com/codedeploy/", Proficiency: "advanced"},
				{ID: 127, Name: "CodePipeline", IconPath: "/static/images/skills/aws_codepipeline.svg", Link: "https://aws.amazon.com/codepipeline/", Proficiency: "advanced"},
				{ID: 22, Name: "GitHub Actions", IconPath: "/static/images/skills/github_actions.svg", Link: "https://github.com/features/actions", Proficiency: "expert", Featured: true, Description: "CI/CD platform for automating build, test, and deployment workflows"},
				{ID: 24, Name: "Jenkins", IconPath: "/static/images/skills/jenkins.svg", Link: "https://www.jenkins.io/", Proficiency: "advanced"},
				{ID: 28, Name: "Packer", IconPath: "/static/images/skills/packer.svg", Link: "https://www.packer.io/", Proficiency: "intermediate"},
				{ID: 103, Name: "Puppet", IconPath: "/static/images/skills/puppet.svg", Link: "https://www.puppet.com/", Proficiency: "intermediate"},
			},
		},
		{
			Name: "Infrastructure as Code",
			Skills: []Skill{
				{ID: 107, Name: "CloudFormation", IconPath: "/static/images/skills/cloudformation.svg", Link: "https://aws.amazon.com/cloudformation/", Proficiency: "expert", Featured: true, Description: "AWS-native infrastructure as code for provisioning cloud resources"},
				{ID: 104, Name: "OpenTofu", IconPath: "/static/images/skills/opentofu.svg", Link: "https://opentofu.org/", Proficiency: "advanced"},
				{ID: 29, Name: "Terraform", IconPath: "/static/images/skills/hashicorp_terraform.svg", Link: "https://www.terraform.io/", Proficiency: "expert", Featured: true, Description: "Multi-cloud infrastructure as code for declarative resource provisioning"},
				{ID: 105, Name: "Terragrunt", IconPath: "/static/images/skills/terragrunt.svg", Link: "https://terragrunt.gruntwork.io/", Proficiency: "advanced"},
				{ID: 106, Name: "Terramate", IconPath: "/static/images/skills/terramate.svg", Link: "https://terramate.io/", Proficiency: "intermediate"},
			},
		},
		{
			Name: "Databases",
			Skills: []Skill{
				{ID: 36, Name: "DynamoDB", IconPath: "/static/images/skills/dynamodb.svg", Link: "https://aws.amazon.com/dynamodb/", Proficiency: "advanced"},
				{ID: 38, Name: "Elasticsearch", IconPath: "/static/images/skills/elasticsearch.svg", Link: "https://www.elastic.co/elasticsearch/", Proficiency: "intermediate"},
				{ID: 34, Name: "MongoDB", IconPath: "/static/images/skills/mongodb.svg", Link: "https://www.mongodb.com/", Proficiency: "intermediate"},
				{ID: 32, Name: "MySQL", IconPath: "/static/images/skills/mysql.svg", Link: "https://www.mysql.com/", Proficiency: "advanced"},
				{ID: 31, Name: "PostgreSQL", IconPath: "/static/images/skills/postgresql.svg", Link: "https://www.postgresql.org/", Proficiency: "advanced"},
				{ID: 35, Name: "Redis", IconPath: "/static/images/skills/redis.svg", Link: "https://redis.io/", Proficiency: "intermediate"},
				{ID: 33, Name: "SQL Server", IconPath: "/static/images/skills/microsoft_sql_server.svg", Link: "https://www.microsoft.com/en-us/sql-server", Proficiency: "advanced"},
				{ID: 37, Name: "SQLite", IconPath: "/static/images/skills/sqlite.svg", Link: "https://www.sqlite.org/", Proficiency: "intermediate"},
			},
		},
		{
			Name: "API & Testing",
			Skills: []Skill{
				{ID: 124, Name: "API Gateway", IconPath: "/static/images/skills/aws_api_gateway.svg", Link: "https://aws.amazon.com/api-gateway/", Proficiency: "advanced"},
				{ID: 39, Name: "FastAPI", IconPath: "/static/images/skills/fastapi.svg", Link: "https://fastapi.tiangolo.com/", Proficiency: "intermediate"},
				{ID: 40, Name: "OpenAPI", IconPath: "/static/images/skills/openapi.svg", Link: "https://www.openapis.org/", Proficiency: "advanced"},
				{ID: 43, Name: "Playwright", IconPath: "/static/images/skills/playwright.svg", Link: "https://playwright.dev/", Proficiency: "advanced"},
				{ID: 41, Name: "Postman", IconPath: "/static/images/skills/postman.svg", Link: "https://www.postman.com/", Proficiency: "advanced"},
				{ID: 42, Name: "pytest", IconPath: "/static/images/skills/pytest.svg", Link: "https://docs.pytest.org/", Proficiency: "advanced"},
			},
		},
		{
			Name: "Development Tools",
			Skills: []Skill{
				{ID: 44, Name: "Git", IconPath: "/static/images/skills/git.svg", Link: "https://git-scm.com/", Proficiency: "expert", Featured: true, Description: "Distributed version control for collaborative development and code management"},
				{ID: 45, Name: "GitHub", IconPath: "/static/images/skills/github.svg", Link: "https://github.com/", Proficiency: "expert"},
				{ID: 46, Name: "GitHub Codespaces", IconPath: "/static/images/skills/github_codespaces.svg", Link: "https://github.com/features/codespaces", Proficiency: "advanced"},
				{ID: 50, Name: "Node.js", IconPath: "/static/images/skills/node.js.svg", Link: "https://nodejs.org/", Proficiency: "advanced"},
				{ID: 49, Name: "npm", IconPath: "/static/images/skills/npm.svg", Link: "https://www.npmjs.com/", Proficiency: "advanced"},
				{ID: 51, Name: "Poetry", IconPath: "/static/images/skills/python_poetry.svg", Link: "https://python-poetry.org/", Proficiency: "advanced"},
				{ID: 52, Name: "Vite", IconPath: "/static/images/skills/vite.js.svg", Link: "https://vitejs.dev/", Proficiency: "intermediate"},
				{ID: 47, Name: "VS Code", IconPath: "/static/images/skills/vscode.svg", Link: "https://code.visualstudio.com/", Proficiency: "expert"},
			},
		},
		{
			Name: "Monitoring & Observability",
			Skills: []Skill{
				{ID: 108, Name: "CloudWatch", IconPath: "/static/images/skills/cloudwatch.svg", Link: "https://aws.amazon.com/cloudwatch/", Proficiency: "expert"},
				{ID: 55, Name: "Datadog", IconPath: "/static/images/skills/datadog.svg", Link: "https://www.datadoghq.com/", Proficiency: "advanced"},
				{ID: 54, Name: "Grafana", IconPath: "/static/images/skills/grafana.svg", Link: "https://grafana.com/", Proficiency: "intermediate"},
				{ID: 53, Name: "Prometheus", IconPath: "/static/images/skills/prometheus.svg", Link: "https://prometheus.io/", Proficiency: "intermediate"},
				{ID: 56, Name: "Splunk", IconPath: "/static/images/skills/splunk.svg", Link: "https://www.splunk.com/", Proficiency: "advanced"},
			},
		},
		{
			Name: "Operating Systems",
			Skills: []Skill{
				{ID: 111, Name: "Debian", IconPath: "/static/images/skills/debian.svg", Link: "https://www.debian.org/", Proficiency: "advanced"},
				{ID: 60, Name: "Raspberry Pi", IconPath: "/static/images/skills/raspberrypi.svg", Link: "https://www.raspberrypi.org/", Proficiency: "intermediate"},
				{ID: 109, Name: "RHEL", IconPath: "/static/images/skills/red_hat.svg", Link: "https://www.redhat.com/en/technologies/linux-platforms/enterprise-linux", Proficiency: "expert"},
				{ID: 110, Name: "Ubuntu", IconPath: "/static/images/skills/ubuntu.svg", Link: "https://ubuntu.com/", Proficiency: "expert"},
				{ID: 59, Name: "Windows", IconPath: "/static/images/skills/windows.svg", Link: "https://www.microsoft.com/windows/", Proficiency: "expert"},
				{ID: 57, Name: "Linux", IconPath: "/static/images/skills/linux.svg", Link: "https://www.linux.org/", Proficiency: "expert", Featured: true, Description: "Primary operating system for servers, containers, and cloud infrastructure"},
			},
		},
		{
			Name: "Web Servers & Frameworks",
			Skills: []Skill{
				{ID: 62, Name: "Apache", IconPath: "/static/images/skills/apache.svg", Link: "https://httpd.apache.org/", Proficiency: "advanced"},
				{ID: 61, Name: "Nginx", IconPath: "/static/images/skills/nginx.svg", Link: "https://nginx.org/", Proficiency: "advanced"},
				{ID: 123, Name: "Amplify", IconPath: "/static/images/skills/aws_amplify.svg", Link: "https://aws.amazon.com/amplify/", Proficiency: "advanced"},
				{ID: 64, Name: "Vue.js", IconPath: "/static/images/skills/vue.js.svg", Link: "https://vuejs.org/", Proficiency: "advanced"},
			},
		},
		{
			Name: "Collaboration Tools",
			Skills: []Skill{
				{ID: 67, Name: "Confluence", IconPath: "/static/images/skills/confluence.svg", Link: "https://www.atlassian.com/software/confluence", Proficiency: "advanced"},
				{ID: 66, Name: "Jira", IconPath: "/static/images/skills/jira.svg", Link: "https://www.atlassian.com/software/jira", Proficiency: "advanced"},
				{ID: 119, Name: "Notion", IconPath: "/static/images/skills/notion.svg", Link: "https://www.notion.so/", Proficiency: "intermediate"},
				{ID: 68, Name: "Slack", IconPath: "/static/images/skills/slack.svg", Link: "https://slack.com/", Proficiency: "expert"},
			},
		},
		{
			Name: "Concepts & Practices",
			Skills: []Skill{
				{ID: 75, Name: "Cloud Architecture", Icon: iconCloudArch, Link: "https://aws.amazon.com/architecture/", Proficiency: "expert"},
				{ID: 71, Name: "Cloud Security", Icon: iconCloudSecurity, Link: "https://www.checkpoint.com/cyber-hub/cloud-security/what-is-cloud-security/", Proficiency: "expert"},
				{ID: 72, Name: "Compliance & Governance", Icon: iconCompliance, Link: "https://www.rapid7.com/fundamentals/compliance-regulatory-frameworks/", Proficiency: "advanced"},
				{ID: 77, Name: "DevSecOps", Icon: iconDevSecOps, Link: "https://www.redhat.com/en/topics/devops/what-is-devsecops", Proficiency: "expert"},
				{ID: 70, Name: "Identity & Access Management", Icon: iconIdentityAccess, Link: "https://www.gartner.com/en/information-technology/glossary/identity-and-access-management-iam", Proficiency: "expert"},
				{ID: 74, Name: "Infrastructure Automation", Icon: iconInfraAuto, Link: "https://www.redhat.com/en/topics/automation/what-is-infrastructure-as-code-iac", Proficiency: "expert"},
				{ID: 76, Name: "Network Security", Icon: iconNetworkSec, Link: "https://www.cisco.com/c/en/us/products/security/what-is-network-security.html", Proficiency: "advanced"},
				{ID: 73, Name: "Observability", Icon: iconMonitoring, Link: "https://newrelic.com/blog/best-practices/what-is-observability", Proficiency: "advanced"},
				{ID: 79, Name: "Security Operations", Icon: iconSecOps, Link: "https://www.microsoft.com/en-us/security/business/security-101/what-is-a-security-operations-center-soc", Proficiency: "advanced"},
				{ID: 78, Name: "Site Reliability Engineering", Icon: iconSRE, Link: "https://sre.google/", Proficiency: "advanced"},
				{ID: 69, Name: "Zero Trust Architecture", Icon: iconZeroTrust, Link: "https://www.cloudflare.com/learning/security/glossary/what-is-zero-trust/", Proficiency: "advanced"},
			},
		},
	}
}

// getFeaturedSkills extracts all featured skills from provided categories
func getFeaturedSkills(categories []SkillCategory) []Skill {
	var featured []Skill
	for _, category := range categories {
		for _, skill := range category.Skills {
			if skill.Featured {
				skill.Category = category.Name
				featured = append(featured, skill)
			}
		}
	}
	return featured
}

func skillsHandler(w http.ResponseWriter, r *http.Request) {
	err := pages.Skills().Render(context.Background(), w)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func skillsGridHandler(w http.ResponseWriter, r *http.Request) {
	categories := skillsData()
	props := partials.SkillsGridProps{
		Categories:     categories,
		FeaturedSkills: getFeaturedSkills(categories),
	}
	err := partials.SkillsGrid(props).Render(context.Background(), w)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func skillsFilteredHandler(w http.ResponseWriter, r *http.Request) {
	categories := skillsData()
	activeCategory := r.URL.Query().Get("category")
	activeProficiency := r.URL.Query().Get("proficiency")

	props := partials.SkillsFilterableProps{
		Categories:        categories,
		ActiveCategory:    activeCategory,
		ActiveProficiency: activeProficiency,
	}
	err := partials.SkillsFilterableSection(props).Render(context.Background(), w)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func skillsDetailHandler(w http.ResponseWriter, r *http.Request) {
	idStr := r.URL.Query().Get("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		http.Error(w, "invalid skill id", http.StatusBadRequest)
		return
	}

	categories := skillsData()
	var found Skill
	var foundCategory string
	for _, cat := range categories {
		for _, skill := range cat.Skills {
			if skill.ID == id {
				found = skill
				foundCategory = cat.Name
				break
			}
		}
		if found.Name != "" {
			break
		}
	}

	if found.Name == "" {
		http.Error(w, "skill not found", http.StatusNotFound)
		return
	}

	found.Category = foundCategory
	props := partials.SkillDetailProps{
		Skill: found,
	}
	err = partials.SkillDetail(props).Render(context.Background(), w)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

/*
========================================
Projects
========================================
*/

// Use types from shared package
type Project = types.Project

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
	err := pages.Projects().Render(context.Background(), w)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func projectsGridHandler(w http.ResponseWriter, r *http.Request) {
	props := partials.ProjectsGridProps{
		Projects: projectsData(),
	}
	err := partials.ProjectsGrid(props).Render(context.Background(), w)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

/*
========================================
Education
========================================
*/

func educationHandler(w http.ResponseWriter, r *http.Request) {
	err := pages.Education().Render(context.Background(), w)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

/*
========================================
Contact
========================================
*/

func contactHandler(w http.ResponseWriter, r *http.Request) {
	err := pages.Contact().Render(context.Background(), w)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

/*
========================================
Soccer
========================================
*/

// Use types from shared package
type Game = types.Game
type LambdaGamesResponse = types.LambdaGamesResponse

func soccerHandler(w http.ResponseWriter, r *http.Request) {
	err := pages.Soccer().Render(context.Background(), w)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func fetchSchedulesHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	_ = r.ParseForm()
	teamCodes := r.FormValue("team_codes")
	props := partials.SoccerTableFragmentProps{
		Games:     mockFetchGames(parseTeamCodes(teamCodes)).Games,
		TeamCodes: teamCodes,
	}
	err := partials.SoccerTableFragment(props).Render(context.Background(), w)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
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
