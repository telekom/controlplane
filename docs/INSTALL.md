Whe# Installation Guide

This guide will help you set up and run both the Slidev presentation and Docusaurus documentation for the controlplane project.

## Prerequisites

Before you begin, ensure you have the following installed:

- **Node.js** (v16.14.0 or higher)
- **npm** (v7.0.0 or higher)
- **Git** (for version control)

## Initial Setup

1. Clone the repository:

```bash
git clone https://github.com/yourusername/controlplane-docs.git
cd controlplane-docs
```

2. Install root dependencies:

```bash
npm install
```

## Slidev Presentation Setup

1. Navigate to the presentation directory:

```bash
cd presentation
```

2. Install dependencies:

```bash
npm install
```

3. Start the development server:

```bash
npm run dev
```

This will open a browser window with the presentation. You can edit the `slides.md` file to update the presentation in real-time.

4. Build the presentation for production:

```bash
npm run build
```

The built presentation will be available in the `presentation/dist` directory.

5. Export to PDF (optional):

```bash
npm run export
```

This will generate a PDF version of the presentation.

## Docusaurus Documentation Setup

1. Navigate to the docusaurus directory:

```bash
cd docusaurus
```

2. Install dependencies:

```bash
npm install
```

3. Start the development server:

```bash
npm run start
```

This will open a browser window with the documentation. You can edit the markdown files in the `docs/docs` directory to update the content.

4. Build the documentation for production:

```bash
npm run build
```

The built documentation will be available in the `docs/build` directory.

5. Serve the production build locally (optional):

```bash
npm run serve
```

## Using the Combined Scripts

For convenience, you can use the scripts defined in the root `package.json`:

- Start Docusaurus development server:

```bash
npm run docs:dev
```

- Start Slidev development server:

```bash
npm run presentation:dev
```

- Build both projects for production:

```bash
npm run docs:build
npm run presentation:build
```

## Deployment

### Deploying the Slidev Presentation

1. Build the presentation:

```bash
npm run presentation:build
```

2. Deploy the `presentation/dist` directory to your web server or hosting service.

### Deploying the Docusaurus Documentation

1. Build the documentation:

```bash
npm run docs:build
```

2. Deploy the `docs/build` directory to your web server or hosting service.

### Automated Deployment

For automated deployment, you can use services like Netlify, Vercel, or GitHub Pages. Here's an example GitHub Actions workflow:

```yaml
name: Deploy Documentation

on:
  push:
    branches: [main]

jobs:
  deploy:
    runs-on: ubuntu-latest
    steps:
      - name: Checkout
        uses: actions/checkout@v3

      - name: Setup Node.js
        uses: actions/setup-node@v3
        with:
          node-version: '16'
          
      - name: Install Dependencies
        run: |
          npm install
          cd presentation && npm install
          cd ../docs && npm install
          
      - name: Build Presentation
        run: cd presentation && npm run build
        
      - name: Build Documentation
        run: cd docs && npm run build
        
      - name: Deploy to GitHub Pages
        uses: JamesIves/github-pages-deploy-action@4.1.4
        with:
          branch: gh-pages
          folder: docs/build
```

## Customization

### Customizing the Slidev Presentation

- Edit `presentation/slides.md` to update the content
- Modify `presentation/style.css` to customize the styling
- Add images to `presentation/public/images`

### Customizing the Docusaurus Documentation

- Update content in `docs/docs/` directory
- Modify `docs/docusaurus.config.js` to change site configuration
- Edit `docs/src/css/custom.css` to customize the styling
- Update `docs/sidebars.js` to change the navigation structure