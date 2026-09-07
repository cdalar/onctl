import type { ReactNode } from 'react';
import clsx from 'clsx';
import Link from '@docusaurus/Link';
import Heading from '@theme/Heading';
import styles from './styles.module.css';

// Deliberately plain, monochrome glyphs (currentColor) rather than each
// provider's own brand logo -- avoids any trademark/licensing question and
// keeps every card visually equal instead of a wall of borrowed logos.
function CloudIcon() {
  return (
    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5" aria-hidden="true">
      <path d="M6.5 19a4.5 4.5 0 0 1-.4-8.98 5.5 5.5 0 0 1 10.7-2A4.5 4.5 0 0 1 17.5 19h-11Z" strokeLinecap="round" strokeLinejoin="round" />
    </svg>
  );
}

function LocalIcon() {
  return (
    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5" aria-hidden="true">
      <rect x="3" y="4" width="18" height="12" rx="1.5" />
      <path d="M2 20h20M9 20l1-4M15 20l-1-4" strokeLinecap="round" strokeLinejoin="round" />
    </svg>
  );
}

type Provider = {
  name: string;
  flag: string;
  kind: 'cloud' | 'local';
  description: string;
};

// Mirrors README.md's own list of supported backends -- kept in sync by
// hand since there's no shared source of truth to import from yet.
const PROVIDERS: Provider[] = [
  { name: 'AWS', flag: 'aws', kind: 'cloud', description: 'EC2 instances' },
  { name: 'Azure', flag: 'azure', kind: 'cloud', description: 'Virtual Machines' },
  { name: 'GCP', flag: 'gcp', kind: 'cloud', description: 'Compute Engine' },
  { name: 'Hetzner', flag: 'hetzner', kind: 'cloud', description: 'Cloud servers' },
  { name: 'Firecracker', flag: 'fc', kind: 'local', description: 'Local microVMs, no cloud account needed' },
  { name: 'Cloud Hypervisor', flag: 'ch', kind: 'local', description: 'Local microVMs, KVM-based' },
];

function ProviderCard({ name, flag, kind, description }: Provider) {
  return (
    <div className={clsx('col col--4', styles.cardCol)}>
      <Link to="/docs/deployments" className={styles.card}>
        <span className={styles.icon}>{kind === 'cloud' ? <CloudIcon /> : <LocalIcon />}</span>
        <Heading as="h3" className={styles.cardTitle}>
          {name}
        </Heading>
        <p className={styles.cardDescription}>{description}</p>
        <code className={styles.cardFlag}>-p {flag}</code>
      </Link>
    </div>
  );
}

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
        <div className="row">
          {PROVIDERS.map((provider) => (
            <ProviderCard key={provider.flag} {...provider} />
          ))}
        </div>
      </div>
    </section>
  );
}
