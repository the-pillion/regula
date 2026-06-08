UPDATE documents
   SET is_publicly_visible = FALSE
 WHERE key IN ('gdpr-rights', 'terms-of-service-passenger', 'terms-of-service-driver');
