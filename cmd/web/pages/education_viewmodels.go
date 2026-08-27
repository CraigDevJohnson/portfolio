package pages

import (
	"fmt"

	"portfolio/cmd/web/partials"
)

type EducationProps struct {
	TotalCerts      int
	Providers       int
	YearsCertifying int
}

type educationCredential struct {
	Href     string
	ImageSrc string
	ImageAlt string
	Title    string
	Provider string
	Year     string
	Loading  string
}

type educationCredentialDomain struct {
	ID          string
	Title       string
	Description string
	Credentials []educationCredential
}

func educationStatCards(props EducationProps) []partials.StatCardProps {
	return []partials.StatCardProps{
		{Value: "1", Label: "Degree", AriaLabel: "1 degree", ExtraClass: "education-stat education-stat-degree stagger-enter"},
		{Value: fmt.Sprintf("%d", props.TotalCerts), Label: "Certifications", AriaLabel: fmt.Sprintf("%d certifications", props.TotalCerts), ExtraClass: "education-stat education-stat-certifications stagger-enter"},
		{Value: fmt.Sprintf("%d", props.Providers), Label: "Providers", AriaLabel: fmt.Sprintf("%d providers", props.Providers), ExtraClass: "education-stat education-stat-providers stagger-enter"},
		{Value: fmt.Sprintf("%d", props.YearsCertifying), Label: "Years Certifying", AriaLabel: fmt.Sprintf("%d years certifying", props.YearsCertifying), ExtraClass: "education-stat education-stat-years stagger-enter"},
	}
}

func educationCredentialDomains() []educationCredentialDomain {
	return []educationCredentialDomain{
		{
			ID:          "cloud",
			Title:       "Cloud",
			Description: "AWS, Azure, and vendor-neutral cloud foundations",
			Credentials: []educationCredential{
				{Href: "https://www.credly.com/badges/c35d4426-9abe-4965-9eb2-9599498e1b5e/", ImageSrc: "/static/images/certs/awscloudpract.png", ImageAlt: "AWS Certified Cloud Practitioner", Title: "AWS Certified Cloud Practitioner", Provider: "Amazon Web Services", Year: "2023", Loading: "eager"},
				{Href: "https://www.credly.com/badges/8bbdc1c8-71d9-4764-b47f-b9f66de1b103", ImageSrc: "/static/images/certs/azure.png", ImageAlt: "Microsoft Certified: Azure Fundamentals", Title: "Microsoft Certified: Azure Fundamentals", Provider: "Microsoft", Year: "2020", Loading: "lazy"},
				{Href: "https://www.credly.com/badges/72ce956a-0c40-485b-8af6-52f7d8652c71", ImageSrc: "/static/images/certs/CompTIA_Cloud_2Bce.png", ImageAlt: "CompTIA Cloud+ ce Certification", Title: "CompTIA Cloud+ ce", Provider: "CompTIA", Year: "2023", Loading: "lazy"},
			},
		},
		{
			ID:          "microsoft",
			Title:       "Microsoft",
			Description: "Enterprise systems credentials",
			Credentials: []educationCredential{
				{Href: "https://www.credly.com/badges/c5406a0d-c4d2-41f7-b897-e12c01182f9f", ImageSrc: "/static/images/certs/MCSE-Cloud-Platform-Infrastructure-2018.png", ImageAlt: "MCSE: Cloud Platform and Infrastructure", Title: "MCSE: Cloud Platform and Infrastructure", Provider: "Microsoft", Year: "2018", Loading: "lazy"},
				{Href: "https://www.credly.com/badges/c2187483-a746-4268-be1a-ea0e5b9281a1", ImageSrc: "/static/images/certs/MCSA-Windows-Server-2016-2018.png", ImageAlt: "MCSA: Windows Server 2016", Title: "MCSA: Windows Server 2016", Provider: "Microsoft", Year: "2018", Loading: "lazy"},
			},
		},
		{
			ID:          "linux",
			Title:       "Linux",
			Description: "Open systems foundation",
			Credentials: []educationCredential{
				{Href: "https://www.credly.com/badges/f8416144-c1cb-4388-bc53-8f622fc7e302", ImageSrc: "/static/images/certs/lpi.png", ImageAlt: "Linux Essentials Certificate", Title: "Linux Essentials Certificate", Provider: "Linux Professional Institute", Year: "2021", Loading: "lazy"},
			},
		},
		{
			ID:          "security",
			Title:       "Security",
			Description: "Infrastructure security fundamentals",
			Credentials: []educationCredential{
				{Href: "https://www.credly.com/badges/276cd921-2315-4136-ae6a-342b477ab6d5", ImageSrc: "/static/images/certs/CompTIA_Security_2Bce.png", ImageAlt: "CompTIA Security+ ce Certification", Title: "CompTIA Security+ ce", Provider: "CompTIA", Year: "2022", Loading: "lazy"},
			},
		},
		{
			ID:          "delivery",
			Title:       "Delivery",
			Description: "Networking, project delivery, and support",
			Credentials: []educationCredential{
				{Href: "https://www.credly.com/badges/4eca7af7-c7e3-4ad5-ad95-b90379c33e52", ImageSrc: "/static/images/certs/CompTIA_Network_2Bce.png", ImageAlt: "CompTIA Network+ ce Certification", Title: "CompTIA Network+ ce", Provider: "CompTIA", Year: "2021", Loading: "lazy"},
				{Href: "https://www.credly.com/badges/a3c22ccb-3e16-4564-af65-c540ae38e99d", ImageSrc: "/static/images/certs/CompTIA_Project_2B.png", ImageAlt: "CompTIA Project+ Certification", Title: "CompTIA Project+", Provider: "CompTIA", Year: "2022", Loading: "lazy"},
				{Href: "https://www.credly.com/badges/b4c88402-4ce7-4d98-a6dd-d0a468d4ac15", ImageSrc: "/static/images/certs/CompTIA_A_2Bce.png", ImageAlt: "CompTIA A+ ce Certification", Title: "CompTIA A+ ce", Provider: "CompTIA", Year: "2021", Loading: "lazy"},
			},
		},
	}
}
