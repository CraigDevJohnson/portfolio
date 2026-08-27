package pages

import (
	"reflect"
	"testing"
)

func TestEducationCredentialDomainsPreserveExplicitAssignments(t *testing.T) {
	want := []struct {
		id          string
		title       string
		credentials []string
	}{
		{id: "cloud", title: "Cloud", credentials: []string{
			"AWS Certified Cloud Practitioner",
			"Microsoft Certified: Azure Fundamentals",
			"CompTIA Cloud+ ce",
		}},
		{id: "microsoft", title: "Microsoft", credentials: []string{
			"MCSE: Cloud Platform and Infrastructure",
			"MCSA: Windows Server 2016",
		}},
		{id: "linux", title: "Linux", credentials: []string{
			"Linux Essentials Certificate",
		}},
		{id: "security", title: "Security", credentials: []string{
			"CompTIA Security+ ce",
		}},
		{id: "delivery", title: "Delivery", credentials: []string{
			"CompTIA Network+ ce",
			"CompTIA Project+",
			"CompTIA A+ ce",
		}},
	}

	domains := educationCredentialDomains()
	if len(domains) != len(want) {
		t.Fatalf("educationCredentialDomains() domain count = %d, want %d", len(domains), len(want))
	}

	seenTitles := make(map[string]string)
	for index, expected := range want {
		domain := domains[index]
		if domain.ID != expected.id {
			t.Errorf("domain %d ID = %q, want %q", index, domain.ID, expected.id)
		}
		if domain.Title != expected.title {
			t.Errorf("domain %q title = %q, want %q", expected.id, domain.Title, expected.title)
		}

		gotTitles := make([]string, 0, len(domain.Credentials))
		for _, credential := range domain.Credentials {
			gotTitles = append(gotTitles, credential.Title)
			if previousDomain, exists := seenTitles[credential.Title]; exists {
				t.Errorf("credential %q appears in both %q and %q", credential.Title, previousDomain, domain.ID)
			}
			seenTitles[credential.Title] = domain.ID
		}
		if !reflect.DeepEqual(gotTitles, expected.credentials) {
			t.Errorf("domain %q credential titles = %#v, want %#v", expected.id, gotTitles, expected.credentials)
		}
	}

	if len(seenTitles) != 10 {
		t.Errorf("unique credential count = %d, want 10", len(seenTitles))
	}
}

func TestEducationCredentialDomainsSetOneEagerImageThenNineLazyImages(t *testing.T) {
	domains := educationCredentialDomains()
	var loadOrder []string
	for _, domain := range domains {
		for _, credential := range domain.Credentials {
			loadOrder = append(loadOrder, credential.Loading)
		}
	}

	want := []string{"eager", "lazy", "lazy", "lazy", "lazy", "lazy", "lazy", "lazy", "lazy", "lazy"}
	if !reflect.DeepEqual(loadOrder, want) {
		t.Fatalf("credential loading order = %#v, want %#v", loadOrder, want)
	}
}

func TestEducationCredentialDomainsPreserveDestinationAndIdentityContent(t *testing.T) {
	domains := educationCredentialDomains()
	credentials := make(map[string]educationCredential)
	for _, domain := range domains {
		for _, credential := range domain.Credentials {
			credentials[credential.Title] = credential
		}
	}

	want := map[string]educationCredential{
		"AWS Certified Cloud Practitioner":        {Href: "https://www.credly.com/badges/c35d4426-9abe-4965-9eb2-9599498e1b5e/", ImageSrc: "/static/images/certs/awscloudpract.png", ImageAlt: "AWS Certified Cloud Practitioner", Provider: "Amazon Web Services", Year: "2023"},
		"Microsoft Certified: Azure Fundamentals": {Href: "https://www.credly.com/badges/8bbdc1c8-71d9-4764-b47f-b9f66de1b103", ImageSrc: "/static/images/certs/azure.png", ImageAlt: "Microsoft Certified: Azure Fundamentals", Provider: "Microsoft", Year: "2020"},
		"MCSE: Cloud Platform and Infrastructure": {Href: "https://www.credly.com/badges/c5406a0d-c4d2-41f7-b897-e12c01182f9f", ImageSrc: "/static/images/certs/MCSE-Cloud-Platform-Infrastructure-2018.png", ImageAlt: "MCSE: Cloud Platform and Infrastructure", Provider: "Microsoft", Year: "2018"},
		"MCSA: Windows Server 2016":               {Href: "https://www.credly.com/badges/c2187483-a746-4268-be1a-ea0e5b9281a1", ImageSrc: "/static/images/certs/MCSA-Windows-Server-2016-2018.png", ImageAlt: "MCSA: Windows Server 2016", Provider: "Microsoft", Year: "2018"},
		"Linux Essentials Certificate":            {Href: "https://www.credly.com/badges/f8416144-c1cb-4388-bc53-8f622fc7e302", ImageSrc: "/static/images/certs/lpi.png", ImageAlt: "Linux Essentials Certificate", Provider: "Linux Professional Institute", Year: "2021"},
		"CompTIA Cloud+ ce":                       {Href: "https://www.credly.com/badges/72ce956a-0c40-485b-8af6-52f7d8652c71", ImageSrc: "/static/images/certs/CompTIA_Cloud_2Bce.png", ImageAlt: "CompTIA Cloud+ ce Certification", Provider: "CompTIA", Year: "2023"},
		"CompTIA Security+ ce":                    {Href: "https://www.credly.com/badges/276cd921-2315-4136-ae6a-342b477ab6d5", ImageSrc: "/static/images/certs/CompTIA_Security_2Bce.png", ImageAlt: "CompTIA Security+ ce Certification", Provider: "CompTIA", Year: "2022"},
		"CompTIA Network+ ce":                     {Href: "https://www.credly.com/badges/4eca7af7-c7e3-4ad5-ad95-b90379c33e52", ImageSrc: "/static/images/certs/CompTIA_Network_2Bce.png", ImageAlt: "CompTIA Network+ ce Certification", Provider: "CompTIA", Year: "2021"},
		"CompTIA Project+":                        {Href: "https://www.credly.com/badges/a3c22ccb-3e16-4564-af65-c540ae38e99d", ImageSrc: "/static/images/certs/CompTIA_Project_2B.png", ImageAlt: "CompTIA Project+ Certification", Provider: "CompTIA", Year: "2022"},
		"CompTIA A+ ce":                           {Href: "https://www.credly.com/badges/b4c88402-4ce7-4d98-a6dd-d0a468d4ac15", ImageSrc: "/static/images/certs/CompTIA_A_2Bce.png", ImageAlt: "CompTIA A+ ce Certification", Provider: "CompTIA", Year: "2021"},
	}

	if len(credentials) != len(want) {
		t.Fatalf("credential identity count = %d, want %d", len(credentials), len(want))
	}
	for title, expected := range want {
		credential, ok := credentials[title]
		if !ok {
			t.Errorf("credential %q is missing", title)
			continue
		}
		credential.Title = ""
		credential.Loading = ""
		if !reflect.DeepEqual(credential, expected) {
			t.Errorf("credential %q identity = %#v, want %#v", title, credential, expected)
		}
	}
}
