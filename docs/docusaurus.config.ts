import {themes as prismThemes} from 'prism-react-renderer';
import type {Config} from '@docusaurus/types';
import type * as Preset from '@docusaurus/preset-classic';
// @ts-expect-error -- untyped remark plugin
import remarkCrdReference from './src/plugins/remark-crd-reference.mjs';


const config: Config = {
  title: 'Control Plane',
  tagline:
    'A cloud-agnostic orchestration platform for self-service API management across multi-cloud environments',
  favicon: 'img/eni-tardis.svg',

  future: {
    v4: true,
  },

  url: 'https://telekom.github.io',
  baseUrl: '/controlplane/',

  organizationName: 'telekom',
  projectName: 'controlplane',

  onBrokenLinks: 'throw',

  markdown: {
    hooks: {
      onBrokenMarkdownLinks: 'warn',
    },
    mermaid: true,
  },

  i18n: {
    defaultLocale: 'en',
    locales: ['en'],
  },

  plugins: ['docusaurus-plugin-image-zoom'],

  themes: [
    '@docusaurus/theme-mermaid',
    [
      '@easyops-cn/docusaurus-search-local',
      {
        hashed: true,
        language: ['en'],
        indexBlog: false,
        docsRouteBasePath: '/docs',
      },
    ],
  ],

  presets: [
    [
      'classic',
      {
        docs: {
          sidebarPath: './sidebars.ts',
          beforeDefaultRemarkPlugins: [remarkCrdReference],
          editUrl:
            'https://github.com/telekom/controlplane/tree/main/docs/',
        },
        blog: false,
        theme: {
          customCss: './src/css/custom.css',
        },
      } satisfies Preset.Options,
    ],
  ],

  themeConfig: {
    image: 'img/eni-tardis.png',
    colorMode: {
      defaultMode: 'light',
      respectPrefersColorScheme: true,
    },
    zoom: {
      selector: '.markdown img',
      background: {
        light: 'rgba(255, 255, 255, 0.95)',
        dark: 'rgba(18, 18, 18, 0.95)',
      },
    },
    navbar: {
      title: 'Control Plane',
      logo: {
        alt: 'Open Telekom Integration Platform Logo',
        src: 'img/eni-tardis.svg',
      },
      items: [
        {
          type: 'docSidebar',
          sidebarId: 'docsSidebar',
          position: 'left',
          label: 'Documentation',
        },
        {
          href: 'https://github.com/telekom/controlplane',
          label: 'GitHub',
          position: 'right',
        },
      ],
    },
    footer: {
      style: 'dark',
      links: [
        {
          title: 'Documentation',
          items: [
            {
              label: 'Overview',
              to: '/docs/overview',
            },
            {
              label: 'Admin Journey',
              to: '/docs/admin-journey/installation',
            },
          ],
        },
        {
          title: 'Community',
          items: [
            {
              label: 'GitHub',
              href: 'https://github.com/telekom/controlplane',
            },
            {
              label: 'Contributing',
              href: 'https://github.com/telekom/controlplane/blob/main/CONTRIBUTING.md',
            },
          ],
        },
      ],
      copyright: `Copyright © ${new Date().getFullYear()} Deutsche Telekom AG. Built with Docusaurus.`,
    },
    prism: {
      theme: prismThemes.github,
      darkTheme: prismThemes.dracula,
      additionalLanguages: ['bash', 'yaml', 'json', 'go', 'diff'],
    },
  } satisfies Preset.ThemeConfig,
};

export default config;
