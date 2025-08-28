import React from 'react';

export type Feature = {
  title: string;
  description: string;
  icon?: string;
  link?: {
    text: string;
    url: string;
  };
};

type FeatureGridProps = {
  features: Feature[];
  columns?: 2 | 3 | 4;
};

/**
 * Grid layout for displaying feature cards
 */
export default function FeatureGrid({
  features,
  columns = 2,
}: FeatureGridProps): JSX.Element {
  // Calculate column width based on number of columns
  const colWidth = 12 / columns;
  const colClass = `col col--${colWidth}`;

  return (
    <div className="container">
      <div className="row">
        {features.map((feature, idx) => (
          <div key={idx} className={colClass}>
            <div className="card margin-bottom--lg">
              <div className="card__header">
                <h3>{feature.icon && <span>{feature.icon} </span>}{feature.title}</h3>
              </div>
              <div className="card__body">
                <p>{feature.description}</p>
              </div>
              {feature.link && (
                <div className="card__footer">
                  <a href={feature.link.url} className="button button--primary button--block">
                    {feature.link.text}
                  </a>
                </div>
              )}
            </div>
          </div>
        ))}
      </div>
    </div>
  );
}