import type { ReactNode } from 'react';
import Heading from '@theme/Heading';
import CodeBlock from '@theme/CodeBlock';
import styles from './styles.module.css';

// Deliberately plain, monochrome glyphs (currentColor) rather than each
// provider's own brand logo -- avoids any trademark/licensing question.
function LocalIcon() {
  return (
    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5" aria-hidden="true">
      <rect x="3" y="4" width="18" height="12" rx="1.5" />
      <path d="M2 20h20M9 20l1-4M15 20l-1-4" strokeLinecap="round" strokeLinejoin="round" />
    </svg>
  );
}

const CLOUD_PROVIDERS = [
  { name: 'AWS', flag: 'aws' },
  { name: 'Azure', flag: 'azure' },
  { name: 'GCP', flag: 'gcp' },
  { name: 'Hetzner', flag: 'hetzner' },
];

// Cloud Hypervisor (-p ch) is also supported, but left off the landing page
// for now -- it's newer and less battle-tested than Firecracker, and the
// docs (getting-started.md/deployments.md) are the more appropriate place
// for an option still finding its feet.
const FIRECRACKER = {
  description:
    "AWS's own microVM tech (it's what Lambda runs on) -- boots in milliseconds, on your own hardware, no cloud account needed.",
  command: 'onctl create -n my-box -p fc',
};

export default function ProviderGrid(): ReactNode {
  return (
    <section className={styles.section}>
      <div className="container">
        <div className="text--center margin-bottom--lg">
          <Heading as="h2">One CLI, every backend</Heading>
          <p className={styles.sectionSubtitle}>
            Same commands, same config shape, whether it's a real cloud account or a
            microVM on your own laptop.
          </p>
        </div>

        <div className={styles.cloudRow}>
          <span className={styles.cloudRowLabel}>Cloud providers</span>
          <ul className={styles.pillList}>
            {CLOUD_PROVIDERS.map((p) => (
              <li key={p.flag} className={styles.pill}>
                {p.name} <code className={styles.pillFlag}>-p {p.flag}</code>
              </li>
            ))}
          </ul>
        </div>

        <div className={styles.localCard}>
          <div className={styles.localCardText}>
            <span className={styles.localLabel}>
              <LocalIcon /> Local microVM
            </span>
            <Heading as="h3" className={styles.localCardTitle}>
              Firecracker
            </Heading>
            <p className={styles.localCardDescription}>{FIRECRACKER.description}</p>
          </div>
          <div className={styles.localCardCommand}>
            <CodeBlock language="bash">{FIRECRACKER.command}</CodeBlock>
          </div>
        </div>
      </div>
    </section>
  );
}
