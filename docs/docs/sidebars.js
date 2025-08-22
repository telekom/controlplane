/**
 * Creating a sidebar enables you to:
 - create an ordered group of docs
 - render a sidebar for each doc of that group
 - provide next/previous navigation

 The sidebars can be generated from the filesystem, or explicitly defined here.

 Create as many sidebars as you want.
 */

/** @type {import('@docusaurus/plugin-content-docs').SidebarsConfig} */
const sidebars = {
  // By default, Docusaurus generates a sidebar from the docs folder structure
  tutorialSidebar: [
    'intro',
    {
      type: 'category',
      label: 'Core Technologies',
      items: ['core-tech/golang', 'core-tech/kubernetes'],
    },
    {
      type: 'category',
      label: 'Web Frameworks',
      items: ['web-frameworks/gofiber'],
    },
    {
      type: 'category',
      label: 'Storage',
      items: ['storage/minio'],
    },
    {
      type: 'category',
      label: 'Testing',
      items: ['testing/testify', 'testing/mockery'],
    },
    {
      type: 'category',
      label: 'Authentication',
      items: ['auth/jwt'],
    },
    {
      type: 'category',
      label: 'Infrastructure',
      items: ['infrastructure/kubernetes', 'infrastructure/helm'],
    },
    {
      type: 'category',
      label: 'Monitoring & Logging',
      items: ['monitoring/zap', 'monitoring/prometheus'],
    },
  ],
};

module.exports = sidebars;