import React from 'react';
import { useLocation } from '@docusaurus/router';
import styles from './styles.module.css';

/**
 * Component to use on pages where you want to suppress the auto-generated title
 * but do not want to use the full PageHeader component
 */
export default function NoAutoTitle(): JSX.Element {
  // We still return a div just to have the component render something
  // The actual hiding is done via CSS in custom.css
  return <div className={styles.noAutoTitleWrapper} />;
}