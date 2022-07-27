#!/usr/bin/env node
import 'source-map-support/register';
import { App } from 'aws-cdk-lib';
import { KITInfrastructure } from '../lib/kit-infrastructure';

const app = new App();
new KITInfrastructure(app, 'KITInfrastructure');
