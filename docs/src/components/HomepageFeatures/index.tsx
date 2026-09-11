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
    title: 'Self-Service API Management',
    Svg: require('@site/static/img/declarative-config.svg').default,
    description: (
      <>
        Empower your users to register, publish, and subscribe to APIs on their
        own through the declarative Rover workflow — backed by configurable
        approval strategies that keep platform teams in control.
      </>
    ),
  },
  {
    title: 'Cloud-Agnostic by Design',
    Svg: require('@site/static/img/api-first.svg').default,
    description: (
      <>
        Deploy across any cloud provider or on-premises environment. The Control
        Plane abstracts away infrastructure differences so your teams work with
        a single, consistent interface — regardless of the underlying gateway or
        identity provider.
      </>
    ),
  },
  {
    title: 'Multi-Cloud Meshing',
    Svg: require('@site/static/img/unified-management.svg').default,
    description: (
      <>
        Connect and orchestrate services across different cloud environments
        seamlessly. Route, secure, and observe API and event traffic across
        zones — whether they run on AWS, Azure, GCP, or on-premises.
      </>
    ),
  },
];

function Feature({title, Svg, description}: FeatureItem) {
  return (
    <div className={clsx('col col--4')}>
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

export default function HomepageFeatures(): ReactNode {
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
