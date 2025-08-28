import React from 'react';

type FeatureCardProps = {
  title: string;
  description: string;
  icon?: string;
};

/**
 * Reusable Feature Card component for consistent feature presentation
 */
export default function FeatureCard({
  title,
  description,
  icon,
}: FeatureCardProps): JSX.Element {
  return (
    <div className="card margin-bottom--lg">
      <div className="card__header">
        <h3>{icon && <span>{icon} </span>}{title}</h3>
      </div>
      <div className="card__body">
        <p>{description}</p>
      </div>
    </div>
  );
}