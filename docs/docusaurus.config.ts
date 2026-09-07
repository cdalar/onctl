import { themes as prismThemes } from 'prism-react-renderer';
import type { Config } from '@docusaurus/types';
import type * as Preset from '@docusaurus/preset-classic';

// This runs in Node.js - Don't use client-side code here (browser APIs, JSX...)

const config: Config = {
  // Used as the browser-tab title suffix ("Getting Started | onctl") --
  // keep this the product name, not marketing copy. The homepage writes
  // its own headline directly in src/pages/index.tsx instead of reusing
  // this.
  title: 'onctl',
  tagline: 'A CLI for VMs -- every cloud, or your own machine',
  favicon: 'img/favicon.ico',

  stylesheets: [
    {
      href: 'https://fonts.googleapis.com/css2?family=JetBrains+Mono:wght@400;500;700;800&family=IBM+Plex+Sans:wght@400;500;600&display=swap',
      type: 'text/css',
    },
  ],

  // Set the production url of your site here
  url: 'https://onctl.sh',
  // Set the /<baseUrl>/ pathname under which your site is served
  // For GitHub pages deployment, it is often '/<projectName>/'
  baseUrl: '/',

  // GitHub pages deployment config.
  // If you aren't using GitHub pages, you don't need these.
  organizationName: 'cdalar', // Usually your GitHub org/user name.
  projectName: 'onctl', // Usually your repo name.

  onBrokenLinks: 'throw',
  markdown: {
    hooks: {
      onBrokenMarkdownLinks: 'warn',
    },
  },

  // Even if you don't use internationalization, you can use this field to set
  // useful metadata like html lang. For example, if your site is Chinese, you
  // may want to replace "en" with "zh-Hans".
  i18n: {
    defaultLocale: 'en',
    locales: ['en'],
  },

  presets: [
    [
      'classic',
      {
        docs: {
          sidebarPath: './sidebars.ts',
          // Please change this to your repo.
          // Remove this to remove the "edit this page" links.
          editUrl:
            'https://github.com/cdalar/onctl/tree/main/docs/',
        },
        // blog: {
        //   showReadingTime: true,
        //   feedOptions: {
        //     type: ['rss', 'atom'],
        //     xslt: true,
        //   },
        //   // Please change this to your repo.
        //   // Remove this to remove the "edit this page" links.
        //   editUrl:
        //     'https://github.com/facebook/docusaurus/tree/main/packages/create-docusaurus/templates/shared/',
        //   // Useful options to enforce blogging best practices
        //   onInlineTags: 'warn',
        //   onInlineAuthors: 'warn',
        //   onUntruncatedBlogPosts: 'warn',
        // },
        blog: false,
        theme: {
          customCss: './src/css/custom.css',
        },
      } satisfies Preset.Options,
    ],
  ],

  plugins: [
    [
      require.resolve('docusaurus-lunr-search'),
      {
        // Options for the plugin (optional)
        languages: ['en'], // Specify languages here if needed
      },
    ],
  ],


  themeConfig: {
    // Replace with your project's social card
    image: 'img/docusaurus-social-card.jpg',
    // onctl's target audience (small-team OSS, technical, CLI-first) skews
    // dark-default among comparable tools -- see LANDING_PAGE_RESEARCH.md.
    // Still fully toggleable and respects the OS preference on first visit.
    colorMode: {
      defaultMode: 'dark',
      respectPrefersColorScheme: true,
    },
    navbar: {
      title: 'onctl',
      logo: {
        alt: 'onctl logo',
        src: 'img/onkube.svg',
      },
      items: [
        {
          type: 'docSidebar',
          sidebarId: 'tutorialSidebar',
          position: 'left',
          label: 'Docs',
        },
        // {to: '/blog', label: 'Blog', position: 'left'},
        {
          href: 'https://github.com/cdalar/onctl',
          label: 'GitHub',
          position: 'right',
        },
      ],
    },
    footer: {
      style: 'dark',
      // links: [
      //   {
      //     title: 'Docs',
      //     items: [
      //       {
      //         label: 'Tutorial',
      //         to: '/docs',
      //       },
      //     ],
      //   },
      // {
      //   title: 'Community',
      //   items: [
      //     {
      //       label: 'Stack Overflow',
      //       href: 'https://stackoverflow.com/questions/tagged/docusaurus',
      //     },
      //     {
      //       label: 'Discord',
      //       href: 'https://discordapp.com/invite/docusaurus',
      //     },
      //     {
      //       label: 'X',
      //       href: 'https://x.com/docusaurus',
      //     },
      //   ],
      // },
      // {
      //   title: 'More',
      //   items: [
      //     {
      //       label: 'Blog',
      //       to: '/blog',
      //     },
      //     {
      //       label: 'GitHub',
      //       href: 'https://github.com/facebook/docusaurus',
      //     },
      //   ],
      // },
      // ],
      copyright: `Copyright © ${new Date().getFullYear()} onctl`,
    },
    prism: {
      theme: prismThemes.github,
      darkTheme: prismThemes.dracula,
    },
  } satisfies Preset.ThemeConfig,
};

export default config;
