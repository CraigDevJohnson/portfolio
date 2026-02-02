# Craig Johnson Portfolio - Go + HTMX

A modern, responsive personal portfolio website built with **Go** and **HTMX** for server-side rendering with dynamic client interactions.

![Go](https://img.shields.io/badge/Go-1.21+-00ADD8?style=flat-square&logo=go&logoColor=white)
![HTMX](https://img.shields.io/badge/HTMX-1.9-3366CC?style=flat-square)

## Overview

This is a complete refactor of the Vue.js portfolio to a server-rendered Go application using HTMX for dynamic interactions. The site maintains the same professional design and functionality while leveraging server-side rendering for improved performance and SEO.

## Features

- **Server-Side Rendering**: Fast initial page loads with Go templates
- **HTMX Interactions**: Dynamic content loading without full page refreshes
- **Dark/Light Theme**: Toggle between themes with persistent preference
- **Responsive Design**: Mobile-first approach with beautiful desktop layouts
- **Professional UI**: Modern design with smooth animations and polish
- **Soccer Tool**: HTMX-powered schedule fetcher with ICS download

## Pages

- **Home** - Hero section with profile, stats, and quick links
- **About** - Personal background and values
- **Experience** - Career timeline with HTMX lazy loading
- **Skills** - Technical proficiencies by category
- **Projects** - Project showcase with links
- **Education** - Academic background and certifications
- **Contact** - Professional contact information
- **Soccer** - Interactive schedule download tool

## Tech Stack

- **Backend**: Go 1.21+
- **Templating**: Go html/template
- **Frontend Interactivity**: HTMX 1.9
- **Styling**: Custom CSS with CSS Variables
- **Fonts**: Inter (Google Fonts)

## Getting Started

### Prerequisites

- Go 1.21 or higher

### Installation

```bash
cd go-portfolio
go build -o portfolio-server .
```

### Running

```bash
./portfolio-server
```

The server will start at `http://localhost:8080`

### Development

For development with hot reload, use `make dev` which requires [air](https://github.com/air-verse/air):

```bash
go install github.com/air-verse/air@latest
make dev
```

## Project Structure

```filetree
go-portfolio/
├── main.go                 # Main application, routes, handlers
├── go.mod                  # Go module definition
├── templates/
│   ├── layouts/
│   │   └── base.html       # Base layout template
│   ├── pages/
│   │   ├── home.html       # Home page
│   │   ├── about.html      # About page
│   │   ├── experience.html # Experience page
│   │   ├── skills.html     # Skills page
│   │   ├── projects.html   # Projects page
│   │   ├── education.html  # Education page
│   │   ├── contact.html    # Contact page
│   │   └── soccer.html     # Soccer tool page
│   └── partials/
│       ├── header.html     # Header partial
│       ├── nav.html        # Navigation partial
│       ├── footer.html     # Footer partial
│       ├── experience_timeline.html
│       ├── skills_grid.html
│       ├── projects_grid.html
│       └── soccer_table_fragment.html
└── static/
    ├── css/
    │   ├── styles.css      # Global styles
    │   ├── home.css        # Home page styles
    │   ├── about.css       # About page styles
    │   └── ...             # Other page styles
    ├── js/
    │   ├── theme.js        # Theme toggle
    │   └── main.js         # Main JavaScript
    └── images/
        └── ...             # Static images
```

## API Endpoints
<!-- markdownlint-disable MD024  -->
### Pages

- `GET /` - Home page
- `GET /about` - About page
- `GET /experience` - Experience page
- `GET /skills` - Skills page
- `GET /projects` - Projects page
- `GET /education` - Education page
- `GET /contact` - Contact page
- `GET /soccer` - Soccer tool page

### HTMX Fragments

- `GET /experience/timeline` - Experience timeline fragment
- `GET /skills/grid` - Skills grid fragment
- `GET /projects/grid` - Projects grid fragment
- `POST /soccer/fetch` - Fetch soccer schedules
- `POST /soccer/download` - Download ICS file
- `POST /soccer/subscribe` - Subscribe to updates

## Design Principles

1. **Progressive Enhancement**: Core content works without JavaScript
2. **HTMX for Interactivity**: Dynamic updates without SPA complexity
3. **Server-Rendered**: Fast initial loads, great SEO
4. **Mobile-First**: Responsive design starting from mobile
5. **Accessible**: Semantic HTML, ARIA labels, keyboard navigation
6. **Themed**: Dark/light mode with CSS variables

## Customization

### Updating Content

Content is defined in `main.go` in the data functions:

- `experienceData()` - Work experience entries
- `skillsData()` - Skills by category
- `projectsData()` - Project showcase
- `educationData()` - Education entries

### Styling

CSS variables are defined in `static/css/styles.css`:

- Colors (light and dark themes)
- Spacing
- Typography
- Shadows
- Animations

## Deployment

The application is designed to be deployed as a standalone binary:

```bash
# Build for production
CGO_ENABLED=0 GOOS=linux go build -o portfolio-server .

# Run
./portfolio-server
```

For containerized deployment, create a Dockerfile:

```dockerfile
FROM golang:1.21-alpine AS builder
WORKDIR /app
COPY . .
RUN go build -o portfolio-server .

FROM alpine:latest
WORKDIR /app
COPY --from=builder /app/portfolio-server .
COPY --from=builder /app/templates ./templates
COPY --from=builder /app/static ./static
EXPOSE 8080
CMD ["./portfolio-server"]
```

## License

MIT License - feel free to use this as a template for your own portfolio!
