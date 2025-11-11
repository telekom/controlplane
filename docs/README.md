# Controlplane Documentation

This directory contains the technical documentation for the Open Telekom Integration Platform's ControlPlane.

## Overview

The ControlPlane documentation is built using **Docusaurus** for comprehensive technical documentation. The documentation provides a high-level overview of the platform architecture, concepts, and usage patterns.

> **Important**: Domain-specific technical details remain in each domain's README.md file. The Docusaurus documentation focuses on general concepts, architecture overview, and cross-domain workflows.

## Documentation Structure

- **Docusaurus (`pages/`)**: General platform overview, architecture, concepts, and getting started guides
- **Slidev (`presentation/`)**: Technical presentations for meetings and conferences
- **Domain READMEs**: Detailed technical documentation for each domain (API, Gateway, Identity, etc.)

## Quick Start

### Prerequisites

- Node.js 20+
- npm 9+

### Installation

```bash
# Navigate to the docs directory
cd docs

# Install dependencies
npm install
```

### Development

**Start Docusaurus development server:**
```bash
npm run docs:dev
# Opens at http://localhost:3000
```

### Building for Production

**Build Docusaurus site:**
```bash
npm run docs:build
npm run docs:serve  # Preview the build
```

**Build Slidev presentation:**
```bash
npm run presentation:build
npm run presentation:export  # Export to PDF
```

## Content Guidelines

### What Goes in Docusaurus

- **Architecture Overview**: High-level system architecture and component interactions
- **Concepts**: Core concepts like Rovers, Zones, Environments, Teams
- **Getting Started**: Installation guides, quick start tutorials
- **Workflows**: Cross-domain workflows (e.g., API lifecycle from Rover to Gateway)
- **Best Practices**: Platform-wide best practices and patterns

### What Stays in Domain READMEs

- **CRD Specifications**: Detailed Custom Resource Definition documentation
- **API References**: Domain-specific API details
- **Implementation Details**: Internal architecture, handlers, controllers
- **Code Examples**: Domain-specific code integration examples
- **Technical Configuration**: Detailed configuration options

## Technology Stack

- **[Docusaurus](https://docusaurus.io/)**: Static site generator for documentation
- **[Slidev](https://sli.dev/)**: Presentation framework for developers
- **[Mermaid](https://mermaid-js.github.io/)**: Diagram and flowchart generation
- **[React](https://reactjs.org/)**: UI framework (used by Docusaurus)

## Contributing to Documentation

1. **For general platform documentation**: Edit files in `docs/pages/docs/`
2. **For domain-specific details**: Edit the respective domain's `README.md`
3. **For presentations**: Edit `presentation/slides.md`

### Adding New Pages

Create a new Markdown file in `pages/docs/` with frontmatter:

```markdown
---
sidebar_position: 1
title: Your Page Title
---

# Your Content Here
```

Update `pages/sidebars.js` if needed for custom navigation.

## Deployment

The documentation is automatically deployed via GitHub Actions on push to the main branch. See `.github/workflows/docs-build.yaml` for details.