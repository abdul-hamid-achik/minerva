import { defineConfig } from 'vitepress'

const siteURL = 'https://minervacli.dev'
const repositoryURL = 'https://github.com/abdul-hamid-achik/minerva'
const defaultDescription =
  'Minerva is the agent library operator and stack readiness CLI/MCP for skills, profiles, and honest intelligence-stack health.'

const structuredData = JSON.stringify({
  '@context': 'https://schema.org',
  '@graph': [
    {
      '@type': 'WebSite',
      '@id': `${siteURL}/#website`,
      url: `${siteURL}/`,
      name: 'Minerva',
      description: defaultDescription,
      inLanguage: 'en-US',
    },
    {
      '@type': 'SoftwareApplication',
      '@id': `${siteURL}/#software`,
      name: 'Minerva',
      url: `${siteURL}/`,
      applicationCategory: 'DeveloperApplication',
      operatingSystem: ['macOS', 'Linux'],
      description: defaultDescription,
      sameAs: repositoryURL,
      author: {
        '@type': 'Person',
        name: 'Abdul Hamid Achik',
      },
      isAccessibleForFree: true,
      offers: {
        '@type': 'Offer',
        price: 0,
        priceCurrency: 'USD',
      },
      featureList: [
        'Skill and profile management for ~/.agents',
        'Tiered stack presence with correct binary names',
        'Fail-closed retrieval readiness (codemap + vecgrep)',
        'MCPHub and Cortex operator intelligence',
        'fcheap evidence tags for closed-loop suggestions',
        'MCP server for agent harnesses',
      ],
    },
  ],
})

export default defineConfig({
  title: 'Minerva',
  titleTemplate: ':title · Minerva',
  description: defaultDescription,
  lang: 'en-US',
  cleanUrls: true,
  lastUpdated: true,
  ignoreDeadLinks: true,

  head: [
    ['link', { rel: 'icon', href: '/favicon.svg', type: 'image/svg+xml' }],
    ['meta', { name: 'theme-color', content: '#191c1b' }],
    ['meta', { property: 'og:type', content: 'website' }],
    ['meta', { property: 'og:site_name', content: 'Minerva' }],
    ['meta', { property: 'og:url', content: siteURL }],
    ['meta', { property: 'og:title', content: 'Minerva' }],
    ['meta', { property: 'og:description', content: defaultDescription }],
    ['meta', { property: 'og:image', content: `${siteURL}/og-minerva.png` }],
    ['meta', { name: 'twitter:card', content: 'summary_large_image' }],
    ['meta', { name: 'twitter:image', content: `${siteURL}/og-minerva.png` }],
    ['script', { type: 'application/ld+json' }, structuredData],
  ],

  themeConfig: {
    logo: { src: '/logo-mark.svg', alt: 'Minerva' },
    siteTitle: 'Minerva',
    nav: [
      { text: 'Start', link: '/guide/getting-started' },
      { text: 'Concepts', link: '/guide/concepts' },
      { text: 'CLI', link: '/guide/cli' },
      { text: 'Stack readiness', link: '/guide/stack' },
      {
        text: 'GitHub',
        link: repositoryURL,
      },
    ],
    sidebar: {
      '/guide/': [
        {
          text: 'Start',
          items: [
            { text: 'Getting started', link: '/guide/getting-started' },
            { text: 'Concepts', link: '/guide/concepts' },
          ],
        },
        {
          text: 'Operate',
          items: [
            { text: 'CLI reference', link: '/guide/cli' },
            { text: 'Stack readiness', link: '/guide/stack' },
            { text: 'Evidence & fcheap', link: '/guide/evidence' },
            { text: 'MCP integration', link: '/guide/mcp' },
          ],
        },
        {
          text: 'Design',
          items: [
            { text: 'Architecture', link: '/guide/architecture' },
            { text: 'Dogfooding (glyph & cairn)', link: '/guide/dogfood' },
          ],
        },
      ],
    },
    socialLinks: [
      { icon: 'github', link: repositoryURL },
    ],
    footer: {
      message: 'Know what is installed. Know what is ready. Act on evidence.',
      copyright: 'Minerva · Open source under MIT',
    },
    search: {
      provider: 'local',
    },
    outline: [2, 3],
  },
})
