import type { ReactNode } from 'react';
import Heading from '@theme/Heading';
import CodeBlock from '@theme/CodeBlock';
import styles from './styles.module.css';

// A real (abbreviated) excerpt of the onctl.yaml `onctl init` actually
// generates -- verified against a live build of the CLI, not illustrative
// filler. Real content in place of a provider-logo grid: the point being
// made ("every backend, one config") is demonstrated rather than claimed.
const CONFIG_EXCERPT = `# onctl.yaml
aws:
  vm:
    type: t2.micro
azure:
  vm:
    type: Standard_D4s_v3
gcp:
  type: n1-standard-1
hetzner:
  vm:
    type: cpx21
# no cloud account needed
fc:
  vcpuCount: 1
  memSizeMib: 2048`;

export default function ConfigPanel(): ReactNode {
  return (
    <section className={styles.section}>
      <div className="container">
        <div className={styles.grid}>
          <div className={styles.text}>
            <Heading as="h2">One config file, every backend</Heading>
            <p>
              <code>onctl init</code> writes a single <code>onctl.yaml</code> with every
              provider's defaults pre-filled. Edit the section you need, then pick it
              with <code>-p</code> or the <code>ONCTL_CLOUD</code> env var -- everything
              else about the command stays the same.
            </p>
            <p className={styles.note}>Abbreviated above -- the real file ships with every field commented.</p>
          </div>
          <div className={styles.code}>
            <CodeBlock language="yaml">{CONFIG_EXCERPT}</CodeBlock>
          </div>
        </div>
      </div>
    </section>
  );
}
