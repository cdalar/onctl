import type { ReactNode } from 'react';
import Link from '@docusaurus/Link';
import useDocusaurusContext from '@docusaurus/useDocusaurusContext';
import Layout from '@theme/Layout';
import Heading from '@theme/Heading';
import CodeBlock from '@theme/CodeBlock';
import ProviderGrid from '@site/src/components/ProviderGrid';
import CommandExamples from '@site/src/components/CommandExamples';

import styles from './index.module.css';

const INSTALL_COMMAND = 'curl -fsSL https://onctl.sh/get.sh | bash';

function HomepageHeader() {
  const { siteConfig } = useDocusaurusContext();
  return (
    <header className={styles.heroBanner}>
      <div className="container">
        <Heading as="h1" className={styles.heroTitle}>
          {siteConfig.title}
        </Heading>
        <p className={styles.heroSubtitle}>{siteConfig.tagline}</p>

        <div className={styles.installBlock}>
          <CodeBlock language="bash">{INSTALL_COMMAND}</CodeBlock>
        </div>

        <div className={styles.buttons}>
          <Link className="button button--primary button--lg" to="/docs/getting-started">
            Get Started
          </Link>
          <Link className="button button--secondary button--lg" to="https://github.com/cdalar/onctl">
            GitHub
          </Link>
        </div>

        <div className={styles.trustBar}>
          <a href="https://github.com/cdalar/onctl/stargazers">
            <img
              src="https://img.shields.io/github/stars/cdalar/onctl?style=social"
              alt="GitHub stars"
              height={20}
            />
          </a>
          <Link to="https://github.com/cdalar/onctl/discussions">Discussions</Link>
          <Link to="https://github.com/cdalar/onctl/issues">Issues</Link>
        </div>
      </div>
    </header>
  );
}

export default function Home(): ReactNode {
  const { siteConfig } = useDocusaurusContext();
  return (
    <Layout
      title={siteConfig.title}
      description="A single CLI to manage VMs across AWS, Azure, GCP, Hetzner, and local Firecracker/Cloud Hypervisor microVMs.">
      <HomepageHeader />
      <main>
        <ProviderGrid />
        <CommandExamples />
      </main>
    </Layout>
  );
}
