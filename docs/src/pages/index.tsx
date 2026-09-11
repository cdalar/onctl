import type { ReactNode } from 'react';
import Link from '@docusaurus/Link';
import Layout from '@theme/Layout';
import Heading from '@theme/Heading';
import TerminalWindow from '@site/src/components/TerminalWindow';
import ConfigPanel from '@site/src/components/ConfigPanel';
import Lifecycle from '@site/src/components/Lifecycle';

import styles from './index.module.css';

function HomepageHeader() {
  return (
    <header className={styles.hero}>
      <div className="container">
        <div className={styles.heroText}>
          <Heading as="h1" className={styles.heroTitle}>
            Six ways to boot a VM.
            <br />
            One command.
          </Heading>
          <p className={styles.heroSubtitle}>
            AWS, Azure, GCP, Hetzner, or a Firecracker microVM on your own machine --
            same CLI, same config file.
          </p>
          <div className={styles.ctaRow}>
            <Link className={styles.ctaPrimary} to="/docs/getting-started">
              Get started
            </Link>
            <Link className={styles.ctaSecondary} to="https://github.com/cdalar/onctl">
              View on GitHub
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

        <div className={styles.heroTerminal}>
          <TerminalWindow
            title="~/onctl"
            showCursor
            lines={[
              { type: 'prompt', text: 'curl -fsSL https://onctl.sh/get.sh | bash' },
              { type: 'prompt', text: 'curl -fsSL https://onctl.sh/get-edge.sh | bash' },
              { type: 'prompt', text: 'onctl create -n my-box -p aws' },
              { type: 'output', text: 'Using: aws', dim: true },
              { type: 'output', text: 'Server IP: 3.68.140.22', dim: true },
              { type: 'output', text: 'Vm started.', dim: true },
              { type: 'prompt', text: 'onctl ssh my-box' },
              { type: 'output', text: 'root@my-box:~#', dim: true },
            ]}
          />
        </div>
      </div>
    </header>
  );
}

export default function Home(): ReactNode {
  return (
    // No `title` prop -- Docusaurus's title formatter always appends
    // " | {siteConfig.title}" to a given title with no special case for
    // title === siteConfig.title, so passing "onctl" here would render
    // the tab title as "onctl | onctl". Omitting it falls back to
    // siteConfig.title alone, which is exactly what the homepage wants.
    <Layout description="A single CLI to manage VMs across AWS, Azure, GCP, Hetzner, and local Firecracker/Cloud Hypervisor microVMs.">
      <HomepageHeader />
      <main>
        <ConfigPanel />
        <Lifecycle />
      </main>
    </Layout>
  );
}
