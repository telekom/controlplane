# Usage Guide

This document explains how to use and maintain the controlplane documentation system, consisting of both a Slidev presentation and Docusaurus documentation.

## Project Structure

The project consists of two main components:

- **Slidev Presentation** (`/presentation`): A technical presentation for meetings and conferences
- **Docusaurus Documentation** (`/docs`): Comprehensive technical documentation

## Using the Slidev Presentation

### Navigation

- Use arrow keys (←/→) to navigate between slides
- Press `o` to view an overview of all slides
- Press `f` to enter fullscreen mode
- Press `p` to enter presenter mode (with notes and preview of next slide)

### Editing Content

1. Open `presentation/slides.md` in your editor
2. Modify the Markdown content
3. The presentation will update in real-time if you're running the development server

### Slide Format

Each slide follows this format:

```markdown
---
layout: default
---

# Slide Title

Slide content goes here

- Bullet points
- More points

```

Special layouts include:
- `image-right`: For slides with an image on the right
- `two-cols`: For slides with two columns
- `statement`: For emphasis slides
- `code-group`: For slides with code examples
- `end`: For the final slide

### Adding Images

1. Place image files in `presentation/public/images/`
2. Reference them in your slides:

```markdown
---
layout: image-right
image: './images/your-image.png'
---

# Slide with Image

Content on the left, image on the right
```

### Exporting

To export the presentation to PDF:

```bash
cd presentation
npm run export
```

The PDF will be saved in the presentation directory.

## Using the Docusaurus Documentation

### Navigation

- Use the sidebar to navigate between sections
- Use the search bar to find specific content
- The navbar contains links to main sections and external resources

### Editing Content

Documentation content is stored in `docs/docs/` as Markdown files:

1. Find the appropriate file in the directory structure
2. Edit the Markdown content
3. The documentation will update in real-time if you're running the development server

### Adding New Pages

1. Create a new Markdown file in the appropriate section directory:

```markdown
---
sidebar_position: 1
---

# Page Title

Content goes here
```

2. Update `docs/sidebars.js` if needed to include the new page in navigation:

```js
module.exports = {
  tutorialSidebar: [
    'intro',
    {
      type: 'category',
      label: 'Your Category',
      items: ['your-category/your-new-page', 'your-category/another-page'],
    },
    // ...
  ],
};
```

### Adding Images

1. Place image files in `docs/static/img/`
2. Reference them in your Markdown:

```markdown
![Alt text](/img/your-image.png)
```

### Adding Code Examples

Use triple backticks with the language specified:

```markdown
​```go
func example() string {
    return "Hello, World!"
}
​```
```

### Adding Diagrams

Use Mermaid syntax for diagrams:

```markdown
​```mermaid
flowchart TD
    A[Start] --> B{Decision}
    B -->|Yes| C[Action]
    B -->|No| D[Another Action]
    C --> E[End]
    D --> E
​```
```

## Updating Dependencies

### Slidev

To update Slidev dependencies:

```bash
cd presentation
npm update
```

### Docusaurus

To update Docusaurus dependencies:

```bash
cd docs
npm update
```

## Adding New Sections

### In the Presentation

1. Edit `presentation/slides.md`
2. Add new slides between the `---` separators

### In the Documentation

1. Create a new directory under `docs/docs/` for the section
2. Add Markdown files to the directory
3. Update `docs/sidebars.js` to include the new section:

```js
module.exports = {
  tutorialSidebar: [
    // ...existing sections
    {
      type: 'category',
      label: 'New Section',
      items: ['new-section/page1', 'new-section/page2'],
    },
  ],
};
```

## Common Tasks

### Updating the Navigation

Edit `docs/docusaurus.config.js` to update the navbar:

```js
navbar: {
  title: 'Controlplane',
  items: [
    // Add or modify items here
    {
      type: 'docSidebar',
      sidebarId: 'tutorialSidebar',
      position: 'left',
      label: 'Documentation',
    },
    {
      href: '/presentation',
      label: 'Presentation',
      position: 'left',
    },
  ],
}
```

### Customizing the Theme

Edit `docs/src/css/custom.css` to update the theme colors:

```css
:root {
  --ifm-color-primary: #e20074; /* Deutsche Telekom magenta */
  --ifm-color-primary-dark: #cb0068;
  /* Add more color variables here */
}
```

### Linking Between Presentation and Documentation

In the presentation (`slides.md`), add links to the documentation:

```markdown
# For More Information

Check out the [detailed documentation](/docs/intro)
```

In the documentation, add links to specific presentation slides (if deployed on the same domain):

```markdown
See the [architecture overview slide](/presentation/#/3) for a visual representation.
```

## Best Practices

1. **Consistent Terminology**: Use the same terms across presentation and documentation
2. **Keep Content in Sync**: Update both presentation and documentation when features change
3. **Regular Updates**: Set a schedule to review and update content
4. **Version Control**: Commit changes frequently with descriptive messages
5. **Testing Links**: Regularly check that all links work correctly
6. **Optimize Images**: Compress images to improve loading times