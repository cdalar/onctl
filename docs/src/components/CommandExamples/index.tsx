import type { ReactNode } from 'react';
import Heading from '@theme/Heading';
import CodeBlock from '@theme/CodeBlock';
import styles from './styles.module.css';

type Example = {
  caption: string;
  command: string;
};

// Real, working commands (mirrors README.md/docs/getting-started.md) --
// not a scripted terminal-replay demo. See LANDING_PAGE_RESEARCH.md: none
// of the comparable CLI sites embed a live terminal in the hero either --
// static real output communicates the same thing without the production
// cost of a recorded demo.
const EXAMPLES: Example[] = [
  {
    caption: 'Boot a VM on a real cloud',
    command: 'onctl create -n my-box -p aws',
  },
  {
    caption: 'Or a local Firecracker microVM -- no cloud account needed',
    command: 'onctl create -n my-box -p fc',
  },
  {
    caption: 'SSH straight in',
    command: 'onctl ssh my-box',
  },
  {
    caption: 'See everything you have running, across every provider',
    command: 'onctl ls',
  },
];

export default function CommandExamples(): ReactNode {
  return (
    <section className={styles.section}>
      <div className="container">
        <div className="text--center margin-bottom--lg">
          <Heading as="h2">Same commands, every provider</Heading>
          <p className={styles.sectionSubtitle}>
            Switch providers with a flag or the <code>ONCTL_CLOUD</code> env var --
            everything else about the workflow stays the same.
          </p>
        </div>
        <div className={styles.grid}>
          {EXAMPLES.map((example) => (
            <div key={example.command} className={styles.exampleCard}>
              <p className={styles.caption}>{example.caption}</p>
              <CodeBlock language="bash">{example.command}</CodeBlock>
            </div>
          ))}
        </div>
      </div>
    </section>
  );
}
