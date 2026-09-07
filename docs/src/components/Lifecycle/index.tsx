import type { ReactNode } from 'react';
import Heading from '@theme/Heading';
import TerminalWindow from '@site/src/components/TerminalWindow';
import styles from './styles.module.css';

export default function Lifecycle(): ReactNode {
  return (
    <section className={styles.section}>
      <div className="container">
        <div className={styles.grid}>
          <div className={styles.text}>
            <Heading as="h2">The whole lifecycle</Heading>
            <p>
              Create, connect, check on it, tear it down -- four verbs, regardless of
              which of the six backends is on the other end.
            </p>
          </div>
          <div className={styles.terminal}>
            <TerminalWindow
              title="~/onctl"
              lines={[
                { type: 'prompt', text: 'onctl create -n my-box -p aws' },
                { type: 'output', text: 'Server IP: 3.68.140.22', dim: true },
                { type: 'prompt', text: 'onctl ssh my-box' },
                { type: 'output', text: 'root@my-box:~#', dim: true },
                { type: 'prompt', text: 'onctl ls' },
                { type: 'output', text: 'CLOUD  NAME    PUBLIC IP     STATE', dim: true },
                { type: 'output', text: 'aws    my-box  3.68.140.22   running', dim: true },
                { type: 'prompt', text: 'onctl destroy my-box' },
                { type: 'output', text: 'my-box destroyed.', dim: true },
              ]}
            />
          </div>
        </div>
      </div>
    </section>
  );
}
