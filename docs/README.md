# Controlplane Technical Documentation

This repository contains technical documentation and presentations for the Open Telekom Integration Platform's controlplane.

## Project Structure

- `docs/` - Comprehensive technical documentation built with Docusaurus
- `presentation/` - Technical presentation built with Slidev

## Getting Started

### Requirements

- Node.js 16+
- npm 7+

### Installation

```bash
# Clone the repository
git clone <repository-url>
cd controlplane-docs

# Install dependencies for both projects
npm install
```

## Development

### Documentation (Docusaurus)

```bash
# Start development server
npm run docs:dev

# Build for production
npm run docs:build

# Serve production build locally
npm run docs:serve
```

### Presentation (Slidev)

```bash
# Start development server
npm run presentation:dev

# Build for production
npm run presentation:build

# Export to PDF
npm run presentation:export
```

## Technology Stack

This documentation project uses:

- [Docusaurus](https://docusaurus.io/) - For comprehensive documentation
- [Slidev](https://sli.dev/) - For technical presentations
- [Mermaid](https://mermaid-js.github.io/) - For diagrams
- [KaTeX](https://katex.org/) - For mathematical notations

## License

Apache-2.0