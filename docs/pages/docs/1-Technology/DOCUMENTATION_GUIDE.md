# Technology Documentation Guide

This guide provides instructions for maintaining consistency in the Control Plane's technology documentation.

## Documentation Template

All technology documentation should follow the standardized structure defined in `_template.mdx`. This ensures a consistent user experience across all documentation pages.

## Required Sections

Every technology documentation file must include these sections in the following order:

1. **Front Matter**
   - `sidebar_position`: Determines ordering in the sidebar
   - `title`: The document title

2. **Component Imports**
   ```jsx
   import PageHeader from '@site/src/components/PageHeader';
   import FeatureCard from '@site/src/components/FeatureCard';
   import CardGrid from '@site/src/components/CardGrid';
   import InfoSection from '@site/src/components/InfoSection';
   import FeatureGrid from '@site/src/components/FeatureGrid';
   import NoAutoTitle from '@site/src/components/NoAutoTitle';
   ```

3. **Page Header**
   ```jsx
   <NoAutoTitle />
   
   <PageHeader 
     title="Technology Name"
     description="Concise one-line description of the technology"
   />
   ```

4. **Introduction**
   - One paragraph explaining the technology and its role in the Control Plane
   - Followed by an `<InfoSection type="info">` providing context

5. **Overview**
   - Comprehensive explanation of the technology's core concepts

6. **Why [Technology]?**
   - Benefits and reasons for using this technology
   - Implemented using `<FeatureGrid>` component

7. **Integration in the Control Plane**
   - Specific implementation details in the Control Plane
   - Include use cases with `<CardGrid>` and `<FeatureCard>` components
   - Use an `<InfoSection type="tip">` for integration insights

8. **Best Practices**
   - Implementation recommendations
   - Use `<FeatureGrid>` to list practices

9. **Related Resources**
   - Links to related technologies using `<CardGrid>` with `<FeatureCard>` components

## Optional Sections

Depending on the technology, include these sections as appropriate:

1. **Configuration**
   - For technologies requiring configuration
   - Include code examples in appropriate language syntax

2. **Advanced Features**
   - For complex technologies with advanced capabilities
   - Use `<FeatureGrid>` or `<CardGrid>` components

3. **Security Considerations**
   - For security-related technologies
   - May include an `<InfoSection type="warning">`

## Component Usage Guidelines

### InfoSection Types

- **info** (blue): For general information and explanations
- **tip** (green): For best practices and helpful advice
- **note** (purple): For important supplementary information
- **warning** (orange): For critical security/data integrity concerns

### FeatureGrid

- Use for concise lists of features, benefits, or practices
- Include emojis in titles for visual appeal
- Keep descriptions concise (1-2 sentences)

### CardGrid and FeatureCard

- Use for more detailed content blocks
- Good for explaining use cases, configurations, or detailed features
- Can include lists, code snippets, and formatted content

## Formatting Guidelines

1. **Headings**
   - Use `##` for main sections (h2)
   - Use `###` for subsections (h3)
   - Use sentence case for all headings

2. **Code Blocks**
   - Always specify the language for syntax highlighting
   - Example: ```go, ```yaml, ```bash

3. **Links**
   - Use relative links for internal documentation
   - Use full URLs for external resources

4. **Images**
   - Center images using `<div align="center">`
   - Include alt text for accessibility
   - Set appropriate width (usually 300-500px)

## Example

For a reference implementation, see any of these well-structured documents:

- `/docs/pages/docs/1-Technology/1-Core-Technologies/golang.mdx`
- `/docs/pages/docs/1-Technology/4-Testing/ginkgo.mdx`
- `/docs/pages/docs/1-Technology/3-Storage/minio.mdx`