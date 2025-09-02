# Logo Usage Guidelines for Technology Documentation

This guide explains how to consistently include technology logos in the documentation.

## Logo Location

All technology logos should be stored locally in the `/static/img/logos/` directory rather than being hotlinked from external URLs. This ensures:

1. Consistent availability and loading speed
2. Protection against external URL changes
3. Avoidance of potential CORS issues
4. Consistent styling and quality

## Adding a New Logo

To add a new technology logo:

1. Find an official SVG or high-quality PNG logo for the technology
2. Add the URL to the `/scripts/download-logos.sh` script
3. Run the script to download the logo
4. Use the logo in your documentation file

## Logo Implementation

When adding a logo to a documentation file, use the following pattern:

```jsx
<div align="center">
  <img src="/img/logos/[technology-name]-logo.svg" alt="[Technology Name] Logo" style={{width: '250px', marginBottom: '2rem'}} />
</div>
```

Place this block immediately after the Overview section and before the "Why [Technology]?" section.

## Logo Naming Convention

Follow these naming conventions for consistency:

- All lowercase
- Hyphenated names (e.g., `gofiber-logo.svg`)
- Include `-logo` suffix
- Prefer SVG format when available, otherwise use PNG

## Image Size Guidelines

For visual consistency:

- Width: 250px for most logos
- Height: Auto (preserve aspect ratio)
- Margin: 2rem bottom margin for proper spacing

## Examples

Here are examples of proper logo implementation:

### Helm Logo

```jsx
<div align="center">
  <img src="/img/logos/helm-logo.svg" alt="Helm Logo" style={{width: '250px', marginBottom: '2rem'}} />
</div>
```

### Prometheus Logo

```jsx
<div align="center">
  <img src="/img/logos/prometheus-logo.svg" alt="Prometheus Logo" style={{width: '250px', marginBottom: '2rem'}} />
</div>
```

## Updating External Logos

If you need to update the local logos with newer versions from official sources:

1. Update the URL in the `/scripts/download-logos.sh` script
2. Run the script again to download the updated logo

This approach ensures all technology documentation pages maintain consistent and reliable logo display.