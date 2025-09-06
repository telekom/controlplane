import type {ReactNode} from 'react';
import clsx from 'clsx';
import Heading from '@theme/Heading';
import styles from './styles.module.css';

type FeatureItem = {
  title: string;
  Svg: React.ComponentType<React.ComponentProps<'svg'>>;
  description: ReactNode;
};

const FeatureList: FeatureItem[] = [
  {
    title: 'Unified Management',
    Svg: require('@site/static/img/unified-management.svg').default,
    description: (
      <>
        Centralized control across multiple Kubernetes clusters with a unified management layer
        that maintains the desired state of your infrastructure and workloads.
      </>
    ),
  },
  {
    title: 'Security by Design',
    Svg: require('@site/static/img/security-design.svg').default,
    description: (
      <>
        Built-in OAuth 2.0 security with fine-grained API access control and integrated
        permission management for robust protection and compliance.
      </>
    ),
  },
  {
    title: 'API-First Approach',
    Svg: require('@site/static/img/api-first.svg').default,
    description: (
      <>
        Complete API lifecycle management with cloud-independent service integration,
        enabling seamless connectivity between systems and services.
      </>
    ),
  },
  {
    title: 'Declarative Configuration',
    Svg: require('@site/static/img/declarative-config.svg').default,
    description: (
      <>
        Define your infrastructure and application requirements as code for consistent,
        repeatable deployments and easier maintenance across environments.
      </>
    ),
  },
];

function Feature({title, Svg, description}: FeatureItem) {
  return (
    <div className={clsx('col col--3')}>
      <div className="text--center">
        <Svg className={styles.featureSvg} role="img" />
      </div>
      <div className="text--center padding-horiz--md">
        <Heading as="h3">{title}</Heading>
        <p>{description}</p>
      </div>
    </div>
  );
}

export default function ControlplaneFeatures(): ReactNode {
  return (
    <section className={styles.features}>
      <div className="container">
        <div className="row">
          {FeatureList.map((props, idx) => (
            <Feature key={idx} {...props} />
          ))}
        </div>
      </div>
    </section>
  );
}
